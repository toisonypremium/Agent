package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
)

func (d *DB) SaveLiveOrderFromParams(clientOrderID, orderID, instID, symbol, side, ordType string, price, quantity, notional float64, status string) error {
	return d.SaveManagedLiveOrder(clientOrderID, orderID, instID, symbol, side, ordType, price, quantity, notional, status, live.OrderStatus{})
}

func (d *DB) SaveManagedLiveOrder(clientOrderID, orderID, instID, symbol, side, ordType string, price, quantity, notional float64, status string, meta live.OrderStatus) error {
	now := time.Now().Unix()
	_, err := d.Exec(
		`INSERT OR REPLACE INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		clientOrderID, orderID, instID, symbol, side, ordType, price, quantity, notional, status, now, now, meta.LayerIndex, meta.Source, meta.InvalidationPrice, meta.ExpiresAt, meta.DecisionReason, meta.LastManagementAction,
	)
	return err
}

func (d *DB) ReserveManagedLiveOrder(clientOrderID string, desired liveguard.ManagedDesiredOrder, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	now := time.Now().Unix()
	expiresAt := int64(0)
	if !desired.ExpiresAt.IsZero() {
		expiresAt = desired.ExpiresAt.Unix()
	}
	_, err := d.Exec(
		`INSERT INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action, decision_id, intent, strategy_version, config_hash) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		clientOrderID, "", desired.InstID, desired.Symbol, desired.Side, desired.Type, desired.Price, desired.Quantity, desired.Notional, live.StatusPlanned, now, now, desired.LayerIndex, desired.Source, desired.InvalidationPrice, expiresAt, desired.DecisionReason, "planned: "+reason, desired.DecisionID, desired.Intent, desired.StrategyVersion, desired.ConfigHash,
	)
	return err
}

func (d *DB) MarkManagedLiveOrderSubmitted(clientOrderID string, result live.OrderResult) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	res, err := d.Exec(`UPDATE live_orders SET order_id=CASE WHEN ?<>'' THEN ? ELSE order_id END, inst_id=CASE WHEN ?<>'' THEN ? ELSE inst_id END, status=?, updated_at=?, last_management_action=? WHERE client_order_id=?`, result.OrderID, result.OrderID, result.InstID, result.InstID, live.StatusSubmitted, time.Now().Unix(), "submitted", clientOrderID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("live order reservation not found: client_order_id=%q", clientOrderID)
	}
	return nil
}

func (d *DB) MarkManagedLiveOrderRejected(clientOrderID string, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	res, err := d.Exec(`UPDATE live_orders SET status=?, updated_at=?, last_management_action=? WHERE client_order_id=?`, live.StatusRejected, time.Now().Unix(), "rejected: "+reason, clientOrderID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("live order reservation not found: client_order_id=%q", clientOrderID)
	}
	return nil
}

func (d *DB) HasManagedRealOrderSubmission() (bool, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM live_orders WHERE status IN ('SUBMITTED', 'PARTIAL_FILL', 'LIVE_OPEN', 'PARTIALLY_FILLED', 'FILLED') AND (source LIKE 'deterministic_agent2_layer_%' OR last_management_action LIKE 'submitted%' OR last_management_action LIKE 'placed:%')`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) OpenLiveOrders() ([]live.OrderStatus, error) {
	return d.OpenLiveOrdersDetailed()
}

func (d *DB) OpenLiveOrdersDetailed() ([]live.OrderStatus, error) {
	rows, err := d.Query(`SELECT o.client_order_id, o.order_id, o.inst_id, o.symbol, o.type, o.side, o.price, o.quantity, o.notional, o.status, o.submitted_at, o.updated_at, o.layer_index, o.source, o.invalidation_price, o.expires_at, o.decision_reason, o.last_management_action, COALESCE(f.filled_quantity,0), COALESCE(f.avg_price,0)
		FROM live_orders o LEFT JOIN live_fills f ON f.client_order_id=o.client_order_id
		WHERE o.status IN ('PLANNED', 'SUBMITTED', 'PARTIAL_FILL', 'LIVE_OPEN', 'PARTIALLY_FILLED')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.OrderStatus{}
	for rows.Next() {
		var o live.OrderStatus
		var symbol, source, decisionReason, lastAction sql.NullString
		var notional, invalidation sql.NullFloat64
		var submittedAt, layerIndex, expiresAt sql.NullInt64
		if err := rows.Scan(&o.ClientOrderID, &o.OrderID, &o.InstID, &symbol, &o.OrderType, &o.Side, &o.Price, &o.Quantity, &notional, &o.Status, &submittedAt, &o.UpdatedAt, &layerIndex, &source, &invalidation, &expiresAt, &decisionReason, &lastAction, &o.FilledQuantity, &o.AvgPrice); err != nil {
			return nil, err
		}
		if symbol.Valid {
			o.Symbol = symbol.String
		}
		if notional.Valid {
			o.Notional = notional.Float64
		}
		if submittedAt.Valid {
			o.SubmittedAt = submittedAt.Int64
		}
		if layerIndex.Valid {
			o.LayerIndex = int(layerIndex.Int64)
		}
		if source.Valid {
			o.Source = source.String
		}
		if invalidation.Valid {
			o.InvalidationPrice = invalidation.Float64
		}
		if expiresAt.Valid {
			o.ExpiresAt = expiresAt.Int64
		}
		if decisionReason.Valid {
			o.DecisionReason = decisionReason.String
		}
		if lastAction.Valid {
			o.LastManagementAction = lastAction.String
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (d *DB) SaveLiveOrderStatus(o live.OrderStatus) error {
	o.Status = live.NormalizeOrderStatus(o.Status)
	updatedAt := o.UpdatedAt
	if updatedAt == 0 {
		updatedAt = time.Now().Unix()
	}
	b, _ := json.Marshal(o)
	update := func(where string, id string) (int64, error) {
		res, err := d.Exec(
			`UPDATE live_orders SET
				order_id=CASE WHEN ?<>'' THEN ? ELSE order_id END,
				inst_id=CASE WHEN ?<>'' THEN ? ELSE inst_id END,
				side=CASE WHEN ?<>'' THEN ? ELSE side END,
				type=CASE WHEN ?<>'' THEN ? ELSE type END,
				price=CASE WHEN ?>0 THEN ? ELSE price END,
				quantity=CASE WHEN ?>0 THEN ? ELSE quantity END,
				notional=CASE WHEN ?>0 AND ?>0 THEN ?*? ELSE notional END,
				status=?, updated_at=?, payload_json=? `+where,
			o.OrderID, o.OrderID,
			o.InstID, o.InstID,
			o.Side, o.Side,
			o.OrderType, o.OrderType,
			o.Price, o.Price,
			o.Quantity, o.Quantity,
			o.Price, o.Quantity, o.Price, o.Quantity,
			o.Status, updatedAt, string(b), id,
		)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	if o.ClientOrderID != "" {
		rows, err := update(`WHERE client_order_id=?`, o.ClientOrderID)
		if err != nil || rows > 0 {
			return err
		}
	}
	if o.OrderID != "" {
		rows, err := update(`WHERE order_id=?`, o.OrderID)
		if err != nil || rows > 0 {
			return err
		}
	}
	return fmt.Errorf("live order status not found: client_order_id=%q order_id=%q", o.ClientOrderID, o.OrderID)
}

func (d *DB) SaveLiveOrderEvent(o live.OrderStatus) error {
	now := time.Now().Unix()
	b, _ := json.Marshal(o)
	_, err := d.Exec(
		`INSERT INTO live_order_events(timestamp, client_order_id, order_id, status, payload_json) VALUES(?,?,?,?,?)`,
		now, o.ClientOrderID, o.OrderID, o.Status, string(b),
	)
	return err
}

func (d *DB) LiveFillSnapshot(clientOrderID, orderID string) (live.LiveFillSnapshot, bool, error) {
	var fill live.LiveFillSnapshot
	var err error
	if clientOrderID != "" {
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at FROM live_fills WHERE client_order_id=?`, clientOrderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt)
	} else if orderID != "" {
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at FROM live_fills WHERE order_id=?`, orderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt)
	} else {
		return live.LiveFillSnapshot{}, false, nil
	}
	if err == sql.ErrNoRows {
		return live.LiveFillSnapshot{}, false, nil
	}
	if err != nil {
		return live.LiveFillSnapshot{}, false, err
	}
	return fill, true, nil
}

func (d *DB) SaveLiveFillSnapshot(fill live.LiveFillSnapshot) error {
	if fill.ClientOrderID == "" {
		return fmt.Errorf("live fill snapshot client_order_id required")
	}
	b, _ := json.Marshal(fill)
	_, err := d.Exec(`INSERT OR REPLACE INTO live_fills(client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at, payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fill.ClientOrderID, fill.OrderID, fill.InstID, fill.Symbol, strings.ToUpper(fill.Side), fill.FilledQuantity, fill.AvgPrice, fill.Fee, strings.ToUpper(fill.FeeCurrency), fill.UpdatedAt, string(b))
	return err
}
