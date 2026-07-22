package storage

import (
	"math"
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
)

func TestLiveLedgerStorage(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	first := live.LivePositionEvent{Timestamp: 100, ClientOrderID: "c1", OrderID: "o1", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: 0.01, FillPrice: 2000, NotionalDelta: 20, FeeDelta: -0.001, FeeCurrency: "USDT", Status: live.StatusPartiallyFilled}
	pos, err := db.ApplyLivePositionEvent(first)
	if err != nil {
		t.Fatalf("apply first event: %v", err)
	}
	first.PositionQty = pos.Quantity
	first.AvgEntryPrice = pos.AvgEntryPrice
	if err := db.SaveLivePositionEvent(first); err != nil {
		t.Fatalf("save first event: %v", err)
	}
	if err := db.SaveLiveFillSnapshot(live.LiveFillSnapshot{ClientOrderID: "c1", OrderID: "o1", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: 0.01, AvgPrice: 2000, Fee: -0.001, FeeCurrency: "USDT", UpdatedAt: 100}); err != nil {
		t.Fatalf("save fill snapshot: %v", err)
	}

	if pos.Symbol != "ETHUSDT" || pos.Quantity != 0.01 || pos.CostBasis != 20 || pos.AvgEntryPrice != 2000 || pos.FeeTotal != -0.001 || pos.FeeCurrency != "USDT" {
		t.Fatalf("bad first position: %+v", pos)
	}

	fill, found, err := db.LiveFillSnapshot("c1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !found || fill.FilledQuantity != 0.01 || fill.FeeCurrency != "USDT" {
		t.Fatalf("bad fill snapshot found=%v fill=%+v", found, fill)
	}

	second := live.LivePositionEvent{Timestamp: 200, ClientOrderID: "c1", OrderID: "o1", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: 0.015, FillPrice: 2100, NotionalDelta: 31.5, FeeDelta: -0.0015, FeeCurrency: "USDT", Status: live.StatusFilled}
	pos, err = db.ApplyLivePositionEvent(second)
	if err != nil {
		t.Fatalf("apply second event: %v", err)
	}
	second.PositionQty = pos.Quantity
	second.AvgEntryPrice = pos.AvgEntryPrice
	if err := db.SaveLivePositionEvent(second); err != nil {
		t.Fatalf("save second event: %v", err)
	}
	if err := db.SaveLiveFillSnapshot(live.LiveFillSnapshot{ClientOrderID: "c1", OrderID: "o1", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: 0.025, AvgPrice: 2060, Fee: -0.0025, FeeCurrency: "USDT", UpdatedAt: 200}); err != nil {
		t.Fatalf("save second fill snapshot: %v", err)
	}

	positions, err := db.LivePositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions len=%d want 1", len(positions))
	}
	pos = positions[0]
	if pos.Quantity != 0.025 || pos.CostBasis != 51.5 || pos.AvgEntryPrice != 2060 || pos.FeeTotal != -0.0025 {
		t.Fatalf("bad final position: %+v", pos)
	}

	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_position_events WHERE symbol=?`, "ETHUSDT").Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("event count=%d want 2", eventCount)
	}
}

func TestLiveLedgerRejectsNegativeSpotPosition(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.ApplyLivePositionEvent(live.LivePositionEvent{Timestamp: 100, InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "SELL", DeltaQuantity: 1, FillPrice: 2000, NotionalDelta: 2000})
	if err == nil {
		t.Fatal("expected negative spot position rejection")
	}
}

func TestSaveLiveFillSnapshotRequiresClientOrderID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SaveLiveFillSnapshot(live.LiveFillSnapshot{OrderID: "o1", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: 0.01}); err == nil {
		t.Fatal("expected missing client_order_id rejection")
	}
}

func TestLiveLedgerFeeCurrencyMixed(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ApplyLivePositionEvent(live.LivePositionEvent{Timestamp: 100, InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: 0.01, FillPrice: 2000, NotionalDelta: 20, FeeDelta: -0.001, FeeCurrency: "USDT"}); err != nil {
		t.Fatal(err)
	}
	pos, err := db.ApplyLivePositionEvent(live.LivePositionEvent{Timestamp: 200, InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: 0.01, FillPrice: 2000, NotionalDelta: 20, FeeDelta: -0.00001, FeeCurrency: "ETH"})
	if err != nil {
		t.Fatal(err)
	}
	if pos.FeeCurrency != "MIXED" {
		t.Fatalf("fee currency=%s want MIXED", pos.FeeCurrency)
	}
}

func TestSaveManagedCycleReport(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveManagedCycleReport(liveguard.ManagedCycleResult{Status: liveguard.ManagedCycleDryRun, Summary: "dry run"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM reports WHERE type='auto_live_management'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("managed cycle reports=%d want 1", count)
	}
}

func TestHermesOwnedPositionsExcludesOtherSourcesAndNetsSells(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "owned.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hermesMeta := live.OrderStatus{Source: "HERMES_OPERATOR"}
	if err := db.SaveManagedLiveOrder("hbuy", "o1", "BTC-USDT", "BTCUSDT", "BUY", "limit", 50000, 0.01, 500, live.StatusFilled, hermesMeta); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveManagedLiveOrder("hsell", "o2", "BTC-USDT", "BTCUSDT", "SELL", "limit", 55000, 0.004, 220, live.StatusFilled, hermesMeta); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveManagedLiveOrder("agent2", "o3", "BTC-USDT", "BTCUSDT", "BUY", "limit", 50000, 1, 50000, live.StatusFilled, live.OrderStatus{Source: "deterministic_agent2_layer_1"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []live.LivePositionEvent{{Timestamp: 1, ClientOrderID: "hbuy", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "BUY", DeltaQuantity: 0.01, NotionalDelta: 500}, {Timestamp: 2, ClientOrderID: "hsell", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "SELL", DeltaQuantity: 0.004, NotionalDelta: 220}, {Timestamp: 3, ClientOrderID: "agent2", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "BUY", DeltaQuantity: 1, NotionalDelta: 50000}} {
		if err := db.SaveLivePositionEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	owned, err := db.HermesOwnedPositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || math.Abs(owned[0].Quantity-0.006) > 1e-12 {
		t.Fatalf("wrong Hermes ownership: %+v", owned)
	}
}
