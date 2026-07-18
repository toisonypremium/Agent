package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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
		`INSERT INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
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

func (d *DB) ReserveManagedLiveOrderWithThesis(clientOrderID string, desired liveguard.ManagedDesiredOrder, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	if strings.TrimSpace(desired.ThesisID) == "" {
		return fmt.Errorf("thesis_id required")
	}
	if strings.ToUpper(strings.TrimSpace(desired.Side)) != "BUY" {
		return fmt.Errorf("thesis reservation requires BUY side")
	}
	if desired.Notional <= 0 || math.IsNaN(desired.Notional) || math.IsInf(desired.Notional, 0) {
		return fmt.Errorf("thesis reservation requires finite positive notional")
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ledger ThesisCapitalLedger
	var created, updated int64
	err = tx.QueryRow(`SELECT thesis_id,symbol,max_exposure_usdt,reserved_usdt,filled_usdt,remaining_dca_usdt,status,version,created_at,updated_at FROM thesis_capital_ledgers WHERE thesis_id=?`, strings.TrimSpace(desired.ThesisID)).Scan(&ledger.ThesisID, &ledger.Symbol, &ledger.MaxExposureUSDT, &ledger.ReservedUSDT, &ledger.FilledUSDT, &ledger.RemainingDCAUSDT, &ledger.Status, &ledger.Version, &created, &updated)
	if err == sql.ErrNoRows {
		return fmt.Errorf("thesis ledger not found: %s", strings.TrimSpace(desired.ThesisID))
	}
	if err != nil {
		return err
	}
	ledger.CreatedAt, ledger.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	if err := ValidateThesisCapitalLedger(ledger); err != nil {
		return err
	}
	if strings.ToUpper(ledger.Symbol) != strings.ToUpper(strings.TrimSpace(desired.Symbol)) {
		return fmt.Errorf("thesis symbol mismatch: ledger=%s order=%s", ledger.Symbol, desired.Symbol)
	}
	switch strings.ToUpper(strings.TrimSpace(ledger.Status)) {
	case "PROBE", "CONFIRMING", "ACCUMULATING":
	default:
		return fmt.Errorf("thesis status does not permit reservation: %s", ledger.Status)
	}
	if ledger.RemainingDCAUSDT+1e-9 < desired.Notional {
		return fmt.Errorf("thesis remaining DCA budget insufficient: remaining=%.8f requested=%.8f", ledger.RemainingDCAUSDT, desired.Notional)
	}
	now := time.Now().Unix()
	expiresAt := int64(0)
	if !desired.ExpiresAt.IsZero() {
		expiresAt = desired.ExpiresAt.Unix()
	}
	_, err = tx.Exec(`INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,layer_index,source,invalidation_price,expires_at,decision_reason,last_management_action,decision_id,intent,strategy_version,config_hash,thesis_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, clientOrderID, "", desired.InstID, desired.Symbol, desired.Side, desired.Type, desired.Price, desired.Quantity, desired.Notional, live.StatusPlanned, now, now, desired.LayerIndex, desired.Source, desired.InvalidationPrice, expiresAt, desired.DecisionReason, "planned: "+reason, desired.DecisionID, desired.Intent, desired.StrategyVersion, desired.ConfigHash, strings.TrimSpace(desired.ThesisID))
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=reserved_usdt+?, remaining_dca_usdt=remaining_dca_usdt-?, updated_at=?, version=version+1 WHERE thesis_id=?`, desired.Notional, desired.Notional, now, strings.TrimSpace(desired.ThesisID))
	if err != nil {
		return err
	}
	return tx.Commit()
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

func (d *DB) MarkManagedLiveOrderUnknown(clientOrderID string, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	res, err := d.Exec(`UPDATE live_orders SET status=?, updated_at=?, last_management_action=? WHERE client_order_id=?`, live.StatusUnknownNeedsManualCheck, time.Now().Unix(), "unknown: "+reason, clientOrderID)
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
	_, _, err := d.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: clientOrderID, Status: live.StatusRejected, UpdatedAt: time.Now().Unix(), LastManagementAction: "rejected: " + reason})
	return err
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
		WHERE o.status IN ('PLANNED', 'SUBMITTED', 'PARTIAL_FILL', 'LIVE_OPEN', 'PARTIALLY_FILLED', 'UNKNOWN_NEEDS_MANUAL_CHECK')`)
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
	if o.Status == live.StatusCancelled || o.Status == live.StatusRejected {
		// Terminal transitions are validated by the release helper below.

		_, _, err := d.SaveTerminalLiveOrderStatusAndRelease(o)
		return err
	}
	var current string
	var lookupErr error
	if o.ClientOrderID != "" {
		lookupErr = d.QueryRow(`SELECT status FROM live_orders WHERE client_order_id=?`, o.ClientOrderID).Scan(&current)
	} else if o.OrderID != "" {
		lookupErr = d.QueryRow(`SELECT status FROM live_orders WHERE order_id=?`, o.OrderID).Scan(&current)
	}
	if lookupErr != nil {
		return fmt.Errorf("live order transition lookup failed: %w", lookupErr)
	}
	if err := ensureLiveOrderTransition(current, o.Status); err != nil {
		return err
	}
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
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at, COALESCE(thesis_id,'') FROM live_fills WHERE client_order_id=?`, clientOrderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt, &fill.ThesisID)
	} else if orderID != "" {
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at, COALESCE(thesis_id,'') FROM live_fills WHERE order_id=?`, orderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt, &fill.ThesisID)
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
	_, err := d.Exec(`INSERT OR REPLACE INTO live_fills(client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at, payload_json, thesis_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, fill.ClientOrderID, fill.OrderID, fill.InstID, fill.Symbol, strings.ToUpper(fill.Side), fill.FilledQuantity, fill.AvgPrice, fill.Fee, strings.ToUpper(fill.FeeCurrency), fill.UpdatedAt, string(b), nullableString(fill.ThesisID))
	return err
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
