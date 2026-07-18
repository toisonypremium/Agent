package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/exchange/live"
)

// SaveTerminalLiveOrderStatusAndRelease persists a confirmed CANCELLED or
// REJECTED status and atomically releases that order's unconsumed thesis
// reservation. Legacy orders are updated without thesis ledger mutation.
func (d *DB) SaveTerminalLiveOrderStatusAndRelease(o live.OrderStatus) (float64, bool, error) {
	o.Status = live.NormalizeOrderStatus(o.Status)
	if o.Status != live.StatusCancelled && o.Status != live.StatusRejected {
		return 0, false, fmt.Errorf("terminal thesis release requires CANCELLED or REJECTED status")
	}
	if strings.TrimSpace(o.ClientOrderID) == "" && strings.TrimSpace(o.OrderID) == "" {
		return 0, false, fmt.Errorf("terminal live order identifier required")
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var clientID, thesisID, side string
	var orderNotional float64
	query := `SELECT client_order_id,COALESCE(thesis_id,''),side,notional FROM live_orders WHERE client_order_id=?`
	id := o.ClientOrderID
	if id == "" {
		query = `SELECT client_order_id,COALESCE(thesis_id,''),side,notional FROM live_orders WHERE order_id=?`
		id = o.OrderID
	}
	if err := tx.QueryRow(query, id).Scan(&clientID, &thesisID, &side, &orderNotional); err != nil {
		return 0, false, err
	}
	updatedAt := o.UpdatedAt
	if updatedAt == 0 {
		updatedAt = time.Now().Unix()
	}
	payload, _ := json.Marshal(o)
	res, err := tx.Exec(`UPDATE live_orders SET order_id=CASE WHEN ?<>'' THEN ? ELSE order_id END,inst_id=CASE WHEN ?<>'' THEN ? ELSE inst_id END,status=?,updated_at=?,last_management_action=CASE WHEN ?<>'' THEN ? ELSE last_management_action END,payload_json=? WHERE client_order_id=?`, o.OrderID, o.OrderID, o.InstID, o.InstID, o.Status, updatedAt, o.LastManagementAction, o.LastManagementAction, string(payload), clientID)
	if err != nil {
		return 0, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows != 1 {
		return 0, false, fmt.Errorf("terminal live order status update did not apply")
	}
	thesisID = strings.TrimSpace(thesisID)
	if thesisID == "" || !strings.EqualFold(side, "BUY") {
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		return 0, true, nil
	}
	var consumed float64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(notional_usdt),0) FROM thesis_capital_events WHERE client_order_id=? AND event_type=?`, clientID, ThesisCapitalEventBuyFill).Scan(&consumed); err != nil {
		return 0, false, err
	}
	release := orderNotional - consumed
	if release < 1e-9 {
		release = 0
	}
	eventKey := fmt.Sprintf("release:%s:%s", clientID, o.Status)
	var existingClient, existingType string
	var existingNotional float64
	err = tx.QueryRow(`SELECT client_order_id,event_type,notional_usdt FROM thesis_capital_events WHERE event_key=?`, eventKey).Scan(&existingClient, &existingType, &existingNotional)
	if err == nil {
		if existingClient != clientID || existingType != ThesisCapitalEventRelease || math.Abs(existingNotional-release) > 1e-9 {
			return 0, false, fmt.Errorf("thesis capital event key collision: %s", eventKey)
		}
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		return release, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	if release > 0 {
		var reserved float64
		if err := tx.QueryRow(`SELECT reserved_usdt FROM thesis_capital_ledgers WHERE thesis_id=?`, thesisID).Scan(&reserved); err != nil {
			return 0, false, err
		}
		if reserved+1e-9 < release {
			return 0, false, fmt.Errorf("terminal release exceeds thesis reserved capital")
		}
	}
	now := time.Now().UTC()
	event := ThesisCapitalEvent{EventKey: eventKey, ThesisID: thesisID, ClientOrderID: clientID, EventType: ThesisCapitalEventRelease, NotionalUSDT: release, CreatedAt: now}
	eventPayload, _ := json.Marshal(event)
	if _, err := tx.Exec(`INSERT INTO thesis_capital_events(event_key,thesis_id,client_order_id,event_type,notional_usdt,created_at,payload_json) VALUES(?,?,?,?,?,?,?)`, eventKey, thesisID, clientID, ThesisCapitalEventRelease, release, now.Unix(), string(eventPayload)); err != nil {
		return 0, false, err
	}
	if release > 0 {
		if _, err := tx.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=reserved_usdt-?,remaining_dca_usdt=remaining_dca_usdt+?,updated_at=?,version=version+1 WHERE thesis_id=?`, release, release, now.Unix(), thesisID); err != nil {
			return 0, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return release, true, nil
}
