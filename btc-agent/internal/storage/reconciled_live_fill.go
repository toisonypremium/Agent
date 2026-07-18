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

// ApplyReconciledLiveFill atomically updates the position, position event,
// cumulative fill snapshot, and thesis capital projection for one fill delta.
func (d *DB) ApplyReconciledLiveFill(event live.LivePositionEvent, snapshot live.LiveFillSnapshot, thesisEventKey string) (live.LivePosition, bool, error) {
	event = normalizeLivePositionEvent(event)
	if err := validateLivePositionEvent(event); err != nil {
		return live.LivePosition{}, false, err
	}
	if strings.TrimSpace(snapshot.ClientOrderID) == "" {
		return live.LivePosition{}, false, fmt.Errorf("fill snapshot client_order_id required")
	}
	if event.ClientOrderID != "" && event.ClientOrderID != snapshot.ClientOrderID {
		return live.LivePosition{}, false, fmt.Errorf("fill event/snapshot client_order_id mismatch")
	}
	if event.ClientOrderID == "" {
		event.ClientOrderID = snapshot.ClientOrderID
	}
	if snapshot.FilledQuantity <= 0 || math.IsNaN(snapshot.FilledQuantity) || math.IsInf(snapshot.FilledQuantity, 0) {
		return live.LivePosition{}, false, fmt.Errorf("cumulative fill quantity must be finite and positive")
	}

	tx, err := d.Begin()
	if err != nil {
		return live.LivePosition{}, false, err
	}
	defer tx.Rollback()
	var previous float64
	err = tx.QueryRow(`SELECT filled_quantity FROM live_fills WHERE client_order_id=?`, snapshot.ClientOrderID).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return live.LivePosition{}, false, err
	}
	if err == nil && math.Abs(previous-snapshot.FilledQuantity) <= 1e-12 {
		pos, _, posErr := livePositionBySymbol(tx, event.Symbol)
		return pos, false, posErr
	}
	if snapshot.FilledQuantity < previous-1e-12 {
		return live.LivePosition{}, false, fmt.Errorf("cumulative fill regressed")
	}
	if math.Abs((snapshot.FilledQuantity-previous)-event.DeltaQuantity) > 1e-9 {
		return live.LivePosition{}, false, fmt.Errorf("fill delta does not match cumulative snapshot")
	}

	pos, err := applyLivePositionEventTx(tx, event)
	if err != nil {
		return live.LivePosition{}, false, err
	}
	event.PositionQty, event.AvgEntryPrice = pos.Quantity, pos.AvgEntryPrice
	eventPayload, _ := json.Marshal(event)
	if _, err := tx.Exec(`INSERT INTO live_position_events(timestamp,client_order_id,order_id,inst_id,symbol,side,delta_quantity,fill_price,notional_delta,fee_delta,fee_currency,position_qty,avg_entry_price,status,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.Timestamp, event.ClientOrderID, event.OrderID, event.InstID, event.Symbol, event.Side, event.DeltaQuantity, event.FillPrice, event.NotionalDelta, event.FeeDelta, event.FeeCurrency, event.PositionQty, event.AvgEntryPrice, event.Status, string(eventPayload)); err != nil {
		return live.LivePosition{}, false, err
	}
	snapshotPayload, _ := json.Marshal(snapshot)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO live_fills(client_order_id,order_id,inst_id,symbol,side,filled_quantity,avg_price,fee,fee_currency,updated_at,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, snapshot.ClientOrderID, snapshot.OrderID, snapshot.InstID, snapshot.Symbol, strings.ToUpper(snapshot.Side), snapshot.FilledQuantity, snapshot.AvgPrice, snapshot.Fee, strings.ToUpper(snapshot.FeeCurrency), snapshot.UpdatedAt, string(snapshotPayload)); err != nil {
		return live.LivePosition{}, false, err
	}

	var thesisID, orderSide, orderSymbol string
	err = tx.QueryRow(`SELECT COALESCE(thesis_id,''),side,symbol FROM live_orders WHERE client_order_id=?`, snapshot.ClientOrderID).Scan(&thesisID, &orderSide, &orderSymbol)
	if err != nil && err != sql.ErrNoRows {
		return live.LivePosition{}, false, err
	}
	if strings.TrimSpace(thesisID) != "" && strings.EqualFold(orderSide, "BUY") {
		if strings.TrimSpace(thesisEventKey) == "" {
			return live.LivePosition{}, false, fmt.Errorf("thesis fill event key required")
		}
		if !strings.EqualFold(orderSymbol, event.Symbol) {
			return live.LivePosition{}, false, fmt.Errorf("thesis order/event symbol mismatch")
		}
		var existingClient, existingType string
		var existingNotional float64
		err = tx.QueryRow(`SELECT client_order_id,event_type,notional_usdt FROM thesis_capital_events WHERE event_key=?`, thesisEventKey).Scan(&existingClient, &existingType, &existingNotional)
		if err == nil {
			return live.LivePosition{}, false, fmt.Errorf("thesis event exists while cumulative snapshot is unapplied")
		}
		if err != sql.ErrNoRows {
			return live.LivePosition{}, false, err
		}
		var reserved float64
		if err := tx.QueryRow(`SELECT reserved_usdt FROM thesis_capital_ledgers WHERE thesis_id=?`, thesisID).Scan(&reserved); err != nil {
			return live.LivePosition{}, false, err
		}
		if reserved+1e-9 < event.NotionalDelta {
			return live.LivePosition{}, false, fmt.Errorf("fill delta exceeds thesis reserved capital")
		}
		now := time.Now().UTC()
		capitalEvent := ThesisCapitalEvent{EventKey: thesisEventKey, ThesisID: thesisID, ClientOrderID: snapshot.ClientOrderID, EventType: ThesisCapitalEventBuyFill, NotionalUSDT: event.NotionalDelta, CreatedAt: now}
		capitalPayload, _ := json.Marshal(capitalEvent)
		if _, err := tx.Exec(`INSERT INTO thesis_capital_events(event_key,thesis_id,client_order_id,event_type,notional_usdt,created_at,payload_json) VALUES(?,?,?,?,?,?,?)`, thesisEventKey, thesisID, snapshot.ClientOrderID, ThesisCapitalEventBuyFill, event.NotionalDelta, now.Unix(), string(capitalPayload)); err != nil {
			return live.LivePosition{}, false, err
		}
		if _, err := tx.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=reserved_usdt-?,filled_usdt=filled_usdt+?,updated_at=?,version=version+1 WHERE thesis_id=?`, event.NotionalDelta, event.NotionalDelta, now.Unix(), thesisID); err != nil {
			return live.LivePosition{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return live.LivePosition{}, false, err
	}
	return pos, true, nil
}
