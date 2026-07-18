package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/exchange/live"
)

func (d *DB) ApplyLivePositionEvent(event live.LivePositionEvent) (live.LivePosition, error) {
	event = normalizeLivePositionEvent(event)
	if err := validateLivePositionEvent(event); err != nil {
		return live.LivePosition{}, err
	}
	tx, err := d.Begin()
	if err != nil {
		return live.LivePosition{}, err
	}
	defer tx.Rollback()
	pos, err := applyLivePositionEventTx(tx, event)
	if err != nil {
		return live.LivePosition{}, err
	}
	if err := tx.Commit(); err != nil {
		return live.LivePosition{}, err
	}
	return pos, nil
}

func normalizeLivePositionEvent(event live.LivePositionEvent) live.LivePositionEvent {
	if event.Symbol == "" {
		event.Symbol = live.InternalSymbol(event.InstID)
	}
	event.Side = strings.ToUpper(event.Side)
	event.FeeCurrency = strings.ToUpper(event.FeeCurrency)
	return event
}

func validateLivePositionEvent(event live.LivePositionEvent) error {
	if event.Symbol == "" {
		return fmt.Errorf("live position event symbol required")
	}
	if event.DeltaQuantity <= 0 {
		return fmt.Errorf("live position event delta quantity must be positive")
	}
	if event.FillPrice <= 0 {
		return fmt.Errorf("live position event fill price must be positive")
	}
	return nil
}

func applyLivePositionEventTx(tx *sql.Tx, event live.LivePositionEvent) (live.LivePosition, error) {
	pos, found, err := livePositionBySymbol(tx, event.Symbol)
	if err != nil {
		return live.LivePosition{}, err
	}
	if found && pos.ThesisID != "" && event.ThesisID != "" && pos.ThesisID != event.ThesisID {
		return live.LivePosition{}, fmt.Errorf("live position thesis conflict: position=%s event=%s", pos.ThesisID, event.ThesisID)
	}
	if pos.ThesisID == "" {
		pos.ThesisID = event.ThesisID
	}
	managed := HermesManagedHolding{}
	var managedAdoptedAt, managedUpdatedAt int64
	_ = tx.QueryRow(`SELECT symbol,inst_id,quantity,avg_entry_price,adopted_at,updated_at,source,COALESCE(thesis_id,'') FROM hermes_managed_holdings WHERE symbol=?`, strings.ToUpper(event.Symbol)).Scan(&managed.Symbol, &managed.InstID, &managed.Quantity, &managed.AvgEntryPrice, &managedAdoptedAt, &managedUpdatedAt, &managed.Source, &managed.ThesisID)
	if managed.ThesisID != "" && event.ThesisID != "" && managed.ThesisID != event.ThesisID {
		return live.LivePosition{}, fmt.Errorf("managed holding thesis conflict: holding=%s event=%s", managed.ThesisID, event.ThesisID)
	}
	if managed.Quantity > pos.Quantity {
		pos = managedPosition(managed)
		found = true
	}
	if !found {
		pos = live.LivePosition{ThesisID: event.ThesisID, Symbol: event.Symbol, InstID: event.InstID}
		if event.Timestamp > 0 {
			pos.OpenedAt = event.Timestamp
		} else {
			pos.OpenedAt = time.Now().Unix()
		}
	}
	if pos.InstID == "" {
		pos.InstID = event.InstID
	}
	switch event.Side {
	case "BUY":
		pos.Quantity += event.DeltaQuantity
		pos.CostBasis += event.NotionalDelta
	case "SELL":
		if pos.Quantity+1e-12 < event.DeltaQuantity {
			return live.LivePosition{}, fmt.Errorf("sell delta %.12f exceeds live position %.12f for %s", event.DeltaQuantity, pos.Quantity, event.Symbol)
		}
		avgCost := 0.0
		if pos.Quantity > 0 {
			avgCost = pos.CostBasis / pos.Quantity
		}
		pos.Quantity -= event.DeltaQuantity
		pos.CostBasis -= avgCost * event.DeltaQuantity
		if pos.Quantity < 1e-12 {
			pos.Quantity = 0
			pos.CostBasis = 0
		}
	default:
		return live.LivePosition{}, fmt.Errorf("unsupported live position side %q", event.Side)
	}
	if event.Side == "SELL" && managed.Symbol != "" {
		remaining := managed.Quantity - event.DeltaQuantity
		if remaining < 1e-12 {
			remaining = 0
		}
		if _, err := tx.Exec(`UPDATE hermes_managed_holdings SET quantity=?,updated_at=? WHERE symbol=?`, remaining, event.Timestamp, managed.Symbol); err != nil {
			return live.LivePosition{}, err
		}
	}
	if pos.Quantity > 0 {
		pos.AvgEntryPrice = pos.CostBasis / pos.Quantity
	} else {
		pos.AvgEntryPrice = 0
	}
	pos.FeeTotal += event.FeeDelta
	pos.FeeCurrency = mergeFeeCurrency(pos.FeeCurrency, event.FeeCurrency)
	if event.Timestamp > 0 {
		pos.UpdatedAt = event.Timestamp
	} else {
		pos.UpdatedAt = time.Now().Unix()
	}
	b, _ := json.Marshal(pos)
	if _, err = tx.Exec(`INSERT OR REPLACE INTO live_positions(symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at, payload_json, thesis_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, pos.Symbol, pos.InstID, pos.Quantity, pos.AvgEntryPrice, pos.CostBasis, pos.FeeTotal, pos.FeeCurrency, pos.UpdatedAt, pos.OpenedAt, string(b), nullableString(pos.ThesisID)); err != nil {
		return live.LivePosition{}, err
	}
	return pos, nil
}

func livePositionBySymbol(q interface {
	QueryRow(string, ...any) *sql.Row
}, symbol string) (live.LivePosition, bool, error) {
	var pos live.LivePosition
	err := q.QueryRow(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at, COALESCE(thesis_id,'') FROM live_positions WHERE symbol=?`, symbol).Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt, &pos.OpenedAt, &pos.ThesisID)
	if err == sql.ErrNoRows {
		return live.LivePosition{}, false, nil
	}
	if err != nil {
		return live.LivePosition{}, false, err
	}
	return pos, true, nil
}

func (d *DB) SaveLivePositionEvent(event live.LivePositionEvent) error {
	b, _ := json.Marshal(event)
	_, err := d.Exec(`INSERT INTO live_position_events(timestamp, client_order_id, order_id, inst_id, symbol, side, delta_quantity, fill_price, notional_delta, fee_delta, fee_currency, position_qty, avg_entry_price, status, payload_json, thesis_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.Timestamp, event.ClientOrderID, event.OrderID, event.InstID, event.Symbol, strings.ToUpper(event.Side), event.DeltaQuantity, event.FillPrice, event.NotionalDelta, event.FeeDelta, strings.ToUpper(event.FeeCurrency), event.PositionQty, event.AvgEntryPrice, event.Status, string(b), nullableString(event.ThesisID))
	return err
}

func (d *DB) LivePositions() ([]live.LivePosition, error) {
	rows, err := d.Query(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at, COALESCE(thesis_id,'') FROM live_positions ORDER BY symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.LivePosition{}
	for rows.Next() {
		var pos live.LivePosition
		if err := rows.Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt, &pos.OpenedAt, &pos.ThesisID); err != nil {
			return nil, err
		}
		out = append(out, pos)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	managed, err := d.HermesManagedHoldings()
	if err != nil {
		return nil, err
	}
	bySymbol := map[string]int{}
	for i := range out {
		bySymbol[strings.ToUpper(out[i].Symbol)] = i
	}
	for _, h := range managed {
		if i, ok := bySymbol[h.Symbol]; ok {
			if h.Quantity > out[i].Quantity {
				out[i] = managedPosition(h)
			}
			continue
		}
		out = append(out, managedPosition(h))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

// HermesOwnedPositions derives net quantity from immutable position events
// whose originating order is explicitly owned by HERMES_OPERATOR.
func (d *DB) HermesOwnedPositions() ([]live.LivePosition, error) {
	rows, err := d.Query(`
		SELECT e.symbol,
		       MAX(e.inst_id),
		       SUM(CASE WHEN UPPER(e.side)='BUY' THEN e.delta_quantity ELSE -e.delta_quantity END),
		       SUM(CASE WHEN UPPER(e.side)='BUY' THEN e.notional_delta ELSE 0 END),
		       MAX(e.timestamp)
		FROM live_position_events e
		JOIN live_orders o ON o.client_order_id=e.client_order_id
		WHERE o.source='HERMES_OPERATOR'
		GROUP BY e.symbol
		HAVING SUM(CASE WHEN UPPER(e.side)='BUY' THEN e.delta_quantity ELSE -e.delta_quantity END) > 0
		ORDER BY e.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.LivePosition{}
	for rows.Next() {
		var pos live.LivePosition
		if err := rows.Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.CostBasis, &pos.UpdatedAt); err != nil {
			return nil, err
		}
		if pos.Quantity > 0 {
			pos.AvgEntryPrice = pos.CostBasis / pos.Quantity
		}
		out = append(out, pos)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	managed, err := d.HermesManagedHoldings()
	if err != nil {
		return nil, err
	}
	bySymbol := map[string]int{}
	for i := range out {
		bySymbol[strings.ToUpper(out[i].Symbol)] = i
	}
	for _, h := range managed {
		if i, ok := bySymbol[h.Symbol]; ok {
			if h.Quantity > out[i].Quantity {
				out[i] = managedPosition(h)
			}
			continue
		}
		out = append(out, managedPosition(h))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

// HermesLastExitAtBySymbol returns the latest persisted Hermes SELL fill per symbol.
// It survives process restarts and is used by the strategy cooldown protection.
func (d *DB) HermesLastExitAtBySymbol() (map[string]time.Time, error) {
	rows, err := d.Query(`SELECT e.symbol, MAX(e.timestamp) FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' AND UPPER(e.side)='SELL' GROUP BY e.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var symbol string
		var ts int64
		if err := rows.Scan(&symbol, &ts); err != nil {
			return nil, err
		}
		if ts > 0 {
			out[strings.ToUpper(symbol)] = time.Unix(ts, 0)
		}
	}
	return out, rows.Err()
}
