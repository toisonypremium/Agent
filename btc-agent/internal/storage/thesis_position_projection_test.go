package storage

import (
	"math"
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestReconciledFillsProjectLifecycleAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lifecycle-projection.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "life", "life-order", 40)
	e := live.LivePositionEvent{Timestamp: 100, ClientOrderID: "life-order", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20, Status: live.StatusPartialFill}
	s := live.LiveFillSnapshot{ClientOrderID: "life-order", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200, UpdatedAt: 100}
	if _, applied, err := db.ApplyReconciledLiveFill(e, s, "life-fill-1"); err != nil || !applied {
		t.Fatalf("first applied=%v err=%v", applied, err)
	}
	got, err := db.ThesisPositionLifecycleByID("life")
	if err != nil || got.State != ThesisPositionProbeOpen || got.PositionQuantity != .1 || got.AvgEntryPrice != 200 {
		t.Fatalf("first lifecycle=%+v err=%v", got, err)
	}
	e.Timestamp = 200
	e.DeltaQuantity = .05
	e.NotionalDelta = 10
	s.FilledQuantity = .15
	s.UpdatedAt = 200
	if _, applied, err := db.ApplyReconciledLiveFill(e, s, "life-fill-2"); err != nil || !applied {
		t.Fatalf("second applied=%v err=%v", applied, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = db.ThesisPositionLifecycleByID("life")
	if err != nil || got.State != ThesisPositionAccumulating || math.Abs(got.PositionQuantity-.15) > 1e-12 {
		t.Fatalf("restart lifecycle=%+v err=%v", got, err)
	}
}

func TestLifecycleProjectionPreservesInvalidatedReview(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "review.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "review", "review-order", 20)
	v := ThesisPositionLifecycle{ThesisID: "review", Symbol: "ETHUSDT", State: ThesisPositionInvalidatedReview}
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	e := live.LivePositionEvent{Timestamp: 1, ClientOrderID: "review-order", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20}
	s := live.LiveFillSnapshot{ClientOrderID: "review-order", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200}
	if _, applied, err := db.ApplyReconciledLiveFill(e, s, "review-fill"); err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	got, err := db.ThesisPositionLifecycleByID("review")
	if err != nil || got.State != ThesisPositionInvalidatedReview || got.PositionQuantity != .1 {
		t.Fatalf("lifecycle=%+v err=%v", got, err)
	}
}

func TestLifecycleProjectionFailureRollsBackFill(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rollback.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "rollback", "rollback-order", 20)
	if _, err := db.Exec(`INSERT INTO thesis_position_lifecycles(thesis_id,symbol,state,version) VALUES('rollback','BTCUSDT','PLANNED',1)`); err != nil {
		t.Fatal(err)
	}
	e := live.LivePositionEvent{Timestamp: 1, ClientOrderID: "rollback-order", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20}
	s := live.LiveFillSnapshot{ClientOrderID: "rollback-order", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200}
	if _, applied, err := db.ApplyReconciledLiveFill(e, s, "rollback-fill"); err == nil || applied {
		t.Fatal("expected lifecycle projection rollback")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_position_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

func TestSellFillClosesThesisLifecycleWithoutCreatingAuthority(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "close.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "close", "buy-close", 20)
	buy := live.LivePositionEvent{Timestamp: 1, ClientOrderID: "buy-close", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20, Status: live.StatusFilled}
	bs := live.LiveFillSnapshot{ClientOrderID: "buy-close", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200}
	if _, _, err := db.ApplyReconciledLiveFill(buy, bs, "buy-close-fill"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveManagedLiveOrder("sell-close", "", "ETH-USDT", "ETHUSDT", "SELL", "limit", 220, .1, 22, live.StatusSubmitted, live.OrderStatus{}); err != nil {
		t.Fatal(err)
	}
	sell := live.LivePositionEvent{Timestamp: 2, ClientOrderID: "sell-close", Symbol: "ETHUSDT", Side: "SELL", DeltaQuantity: .1, FillPrice: 220, NotionalDelta: 22, Status: live.StatusFilled}
	ss := live.LiveFillSnapshot{ClientOrderID: "sell-close", Symbol: "ETHUSDT", Side: "SELL", FilledQuantity: .1, AvgPrice: 220}
	if _, _, err := db.ApplyReconciledLiveFill(sell, ss, ""); err != nil {
		t.Fatal(err)
	}
	got, err := db.ThesisPositionLifecycleByID("close")
	if err != nil || got.State != ThesisPositionClosed || got.PositionQuantity != 0 {
		t.Fatalf("lifecycle=%+v err=%v", got, err)
	}
}
