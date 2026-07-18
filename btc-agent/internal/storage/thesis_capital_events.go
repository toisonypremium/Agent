package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	ThesisCapitalEventBuyFill = "BUY_FILL"
	ThesisCapitalEventRelease = "RELEASE"
)

type ThesisCapitalEvent struct {
	EventKey      string    `json:"event_key"`
	ThesisID      string    `json:"thesis_id"`
	ClientOrderID string    `json:"client_order_id"`
	EventType     string    `json:"event_type"`
	NotionalUSDT  float64   `json:"notional_usdt"`
	CreatedAt     time.Time `json:"created_at"`
}

// ApplyThesisBuyFillDelta atomically journals a unique BUY fill delta and moves
// the same notional from reserved capital to filled capital. Replaying the same
// event key is an idempotent no-op. It does not update positions or fill snapshots.
func (d *DB) ApplyThesisBuyFillDelta(eventKey, clientOrderID string, deltaNotional float64) (bool, error) {
	eventKey = strings.TrimSpace(eventKey)
	clientOrderID = strings.TrimSpace(clientOrderID)
	if eventKey == "" {
		return false, fmt.Errorf("thesis capital event key required")
	}
	if clientOrderID == "" {
		return false, fmt.Errorf("client_order_id required")
	}
	if deltaNotional <= 0 || math.IsNaN(deltaNotional) || math.IsInf(deltaNotional, 0) {
		return false, fmt.Errorf("fill delta notional must be finite and positive")
	}

	tx, err := d.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var existingClientID, existingType string
	var existingNotional float64
	err = tx.QueryRow(`SELECT client_order_id,event_type,notional_usdt FROM thesis_capital_events WHERE event_key=?`, eventKey).Scan(&existingClientID, &existingType, &existingNotional)
	if err == nil {
		if existingClientID != clientOrderID || existingType != ThesisCapitalEventBuyFill || math.Abs(existingNotional-deltaNotional) > 1e-9 {
			return false, fmt.Errorf("thesis capital event key collision: %s", eventKey)
		}
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	var thesisID, symbol, side string
	if err := tx.QueryRow(`SELECT thesis_id,symbol,side FROM live_orders WHERE client_order_id=?`, clientOrderID).Scan(&thesisID, &symbol, &side); err != nil {
		return false, err
	}
	thesisID = strings.TrimSpace(thesisID)
	if thesisID == "" {
		return false, fmt.Errorf("live order has no thesis_id: %s", clientOrderID)
	}
	if !strings.EqualFold(side, "BUY") {
		return false, fmt.Errorf("thesis fill accounting requires BUY order")
	}

	var ledgerSymbol string
	var reserved float64
	if err := tx.QueryRow(`SELECT symbol,reserved_usdt FROM thesis_capital_ledgers WHERE thesis_id=?`, thesisID).Scan(&ledgerSymbol, &reserved); err != nil {
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(symbol), strings.TrimSpace(ledgerSymbol)) {
		return false, fmt.Errorf("thesis fill symbol mismatch: order=%s ledger=%s", symbol, ledgerSymbol)
	}
	if reserved+1e-9 < deltaNotional {
		return false, fmt.Errorf("fill delta exceeds thesis reserved capital: reserved=%.8f delta=%.8f", reserved, deltaNotional)
	}

	now := time.Now().UTC()
	event := ThesisCapitalEvent{EventKey: eventKey, ThesisID: thesisID, ClientOrderID: clientOrderID, EventType: ThesisCapitalEventBuyFill, NotionalUSDT: deltaNotional, CreatedAt: now}
	payload, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`INSERT INTO thesis_capital_events(event_key,thesis_id,client_order_id,event_type,notional_usdt,created_at,payload_json) VALUES(?,?,?,?,?,?,?)`, eventKey, thesisID, clientOrderID, ThesisCapitalEventBuyFill, deltaNotional, now.Unix(), string(payload)); err != nil {
		return false, err
	}
	res, err := tx.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=reserved_usdt-?,filled_usdt=filled_usdt+?,updated_at=?,version=version+1 WHERE thesis_id=? AND reserved_usdt+0.000000001>=?`, deltaNotional, deltaNotional, now.Unix(), thesisID, deltaNotional)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, fmt.Errorf("thesis fill capital update did not apply")
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
