package storage

import (
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestApplyReconciledLiveFillThesisPartialReplayAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic-fill.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "thesis-atomic", "atomic-order", 40)
	event := live.LivePositionEvent{Timestamp: 100, ClientOrderID: "atomic-order", OrderID: "atomic-remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20, FeeCurrency: "USDT", Status: live.StatusPartialFill}
	snapshot := live.LiveFillSnapshot{ClientOrderID: "atomic-order", OrderID: "atomic-remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200, FeeCurrency: "USDT", UpdatedAt: 100}
	pos, applied, err := db.ApplyReconciledLiveFill(event, snapshot, "atomic-fill-1")
	if err != nil || !applied {
		t.Fatalf("first applied=%v err=%v", applied, err)
	}
	if pos.Quantity != .1 {
		t.Fatalf("position=%+v", pos)
	}
	_, applied, err = db.ApplyReconciledLiveFill(event, snapshot, "atomic-fill-1")
	if err != nil || applied {
		t.Fatalf("replay applied=%v err=%v", applied, err)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.ReservedUSDT != 20 || ledger.FilledUSDT != 20 {
		t.Fatalf("ledger=%+v", ledger)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, applied, err = db.ApplyReconciledLiveFill(event, snapshot, "atomic-fill-1")
	if err != nil || applied {
		t.Fatalf("restart replay applied=%v err=%v", applied, err)
	}
}

func TestApplyReconciledLiveFillRollbackOnThesisFailure(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "atomic-rollback.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveManagedLiveOrder("legacy-order", "remote", "ETH-USDT", "ETHUSDT", "BUY", "limit", 200, .1, 20, live.StatusSubmitted, live.OrderStatus{}); err != nil {
		t.Fatal(err)
	}
	event := live.LivePositionEvent{Timestamp: 100, ClientOrderID: "legacy-order", OrderID: "remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20, Status: live.StatusFilled}
	snapshot := live.LiveFillSnapshot{ClientOrderID: "legacy-order", OrderID: "remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200, UpdatedAt: 100}
	if _, applied, err := db.ApplyReconciledLiveFill(event, snapshot, ""); err != nil || !applied {
		t.Fatalf("legacy atomic applied=%v err=%v", applied, err)
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-bad", Symbol: "ETHUSDT", MaxExposureUSDT: 20, ReservedUSDT: 20, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,thesis_id) VALUES('bad-order','bad-remote','ETH-USDT','ETHUSDT','BUY','limit',200,.1,20,'SUBMITTED','thesis-bad')`); err != nil {
		t.Fatal(err)
	}
	badEvent := event
	badEvent.ClientOrderID = "bad-order"
	badEvent.OrderID = "bad-remote"
	badSnapshot := snapshot
	badSnapshot.ClientOrderID = "bad-order"
	badSnapshot.OrderID = "bad-remote"
	if _, applied, err := db.ApplyReconciledLiveFill(badEvent, badSnapshot, ""); err == nil || applied {
		t.Fatal("expected missing thesis event key rollback")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_position_events WHERE client_order_id='bad-order'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled back event count=%d", count)
	}
	if _, found, err := db.LiveFillSnapshot("bad-order", ""); err != nil || found {
		t.Fatalf("rolled back snapshot found=%v err=%v", found, err)
	}
}

func TestApplyReconciledLiveFillSellDoesNotConsumeThesis(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "atomic-sell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: 1, ClientOrderID: "seed", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .2, FillPrice: 200, NotionalDelta: 40}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyLivePositionEvent(live.LivePositionEvent{Timestamp: 1, ClientOrderID: "seed", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .2, FillPrice: 200, NotionalDelta: 40}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveManagedLiveOrder("sell-order", "sell-remote", "ETH-USDT", "ETHUSDT", "SELL", "limit", 210, .1, 21, live.StatusSubmitted, live.OrderStatus{}); err != nil {
		t.Fatal(err)
	}
	event := live.LivePositionEvent{Timestamp: 2, ClientOrderID: "sell-order", OrderID: "sell-remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "SELL", DeltaQuantity: .1, FillPrice: 210, NotionalDelta: 21, Status: live.StatusPartialFill}
	snapshot := live.LiveFillSnapshot{ClientOrderID: "sell-order", OrderID: "sell-remote", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "SELL", FilledQuantity: .1, AvgPrice: 210, UpdatedAt: 2}
	if _, applied, err := db.ApplyReconciledLiveFill(event, snapshot, ""); err != nil || !applied {
		t.Fatalf("sell applied=%v err=%v", applied, err)
	}
}
