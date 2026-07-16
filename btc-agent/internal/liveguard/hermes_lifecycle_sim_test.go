package liveguard_test

import (
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/exchange/simulator"
	"btc-agent/internal/hermesoperator"
	lg "btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"
)

type lifecycleStatusReader struct{ fake *simulator.FakeOKX }

func (r lifecycleStatusReader) OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
	return r.fake.GetOrder(ctx, instID, orderID, clientOrderID)
}
func (lifecycleStatusReader) PendingOrders(context.Context, string) ([]live.OrderStatus, error) {
	return nil, nil
}

func TestHermesSyntheticLifecycleBuyPartialCancelRestartExitLimit(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	fake := simulator.NewFakeOKX()
	fake.SetFilter("BTC-USDT", live.InstrumentFilter{Symbol: "BTCUSDT", InstID: "BTC-USDT", TickSize: .1, StepSize: .001, MinSize: .001, MinNotional: 1})
	fake.SetBalance("USDT", 10000)
	buy := lg.ManagedDesiredOrder{Symbol: "BTCUSDT", InstID: "BTC-USDT", Side: "BUY", Type: "limit", Price: 50000, Quantity: .01, Notional: 500, Source: "HERMES_OPERATOR", DecisionID: "synthetic-buy", Intent: "OPEN_LIMIT"}
	id := "hsyntheticbtc01"
	if err := db.ReserveManagedLiveOrder(id, buy, "synthetic BUY"); err != nil {
		t.Fatal(err)
	}
	placed, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: buy.InstID, Side: "buy", Price: buy.Price, Quantity: buy.Quantity, ClientOrderID: id})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkManagedLiveOrderSubmitted(id, placed); err != nil {
		t.Fatal(err)
	}
	if err := fake.SimFill(id, .004, 50000); err != nil {
		t.Fatal(err)
	}
	status, err := fake.GetOrder(ctx, buy.InstID, placed.OrderID, id)
	if err != nil {
		t.Fatal(err)
	}
	status.Source = "HERMES_OPERATOR"
	prev := live.LiveFillSnapshot{ClientOrderID: id, OrderID: placed.OrderID, InstID: buy.InstID, Symbol: buy.Symbol, Side: "BUY"}
	event, apply, err := lg.BuildPositionEvent(prev, status, time.Unix(1700000000, 0))
	if err != nil || !apply {
		t.Fatalf("buy event apply=%v err=%v", apply, err)
	}
	if _, err := db.ApplyLivePositionEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLivePositionEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLiveFillSnapshot(lg.FillSnapshotFromStatus(status)); err != nil {
		t.Fatal(err)
	}
	cancel := lg.ExecuteHermesCancelActions(ctx, syntheticCfg(), "synthetic-cancel", []lg.HermesActionDecision{syntheticCancelDecision("BTCUSDT")}, []live.OrderStatus{status}, fake, lifecycleStatusReader{fake}, db, false)
	if len(cancel.Canceled) != 1 || cancel.Canceled[0].Order.Status != live.StatusPartialFill {
		t.Fatalf("cancel race lost: %+v", cancel)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owned, err := db.HermesOwnedPositions()
	if err != nil || len(owned) != 1 || owned[0].Quantity != .004 {
		t.Fatalf("restart ownership failed: %+v err=%v", owned, err)
	}
	exit := lg.ExecuteHermesExitLimitActions(ctx, syntheticCfg(), "synthetic-exit", []lg.HermesActionDecision{{Allowed: true, Action: hermesoperator.Action{Symbol: "BTCUSDT", Intent: hermesoperator.IntentExitLimit, RequestedNotionalUSDT: 1000, EntryPrice: 50000}}}, owned, []live.InstrumentFilter{{Symbol: "BTCUSDT", InstID: "BTC-USDT", TickSize: .1, StepSize: .001, MinSize: .001, MinNotional: 1}}, fake, db, false)
	if len(exit.Placed) != 1 || len(exit.Blocked) != 0 || exit.Placed[0].Desired.Quantity != .004 || exit.Placed[0].Desired.Side != "SELL" {
		t.Fatalf("exit lifecycle unsafe: %+v", exit)
	}
}

func syntheticCfg() config.Config {
	c := config.Config{}
	c.HermesOperator.Enabled = true
	c.HermesOperator.Mode = "canary"
	c.Live.Enabled = true
	c.Live.AutoExecute = true
	c.Execution.RealTradingEnabled = true
	return c
}
func syntheticCancelDecision(symbol string) lg.HermesActionDecision {
	return lg.HermesActionDecision{Allowed: true, Action: hermesoperator.Action{Symbol: symbol, Intent: hermesoperator.IntentCancel}}
}

type acceptedTimeoutPlacer struct{ fake *simulator.FakeOKX }

func (p acceptedTimeoutPlacer) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	if _, err := p.fake.PlaceSpotLimitOrder(ctx, req); err != nil {
		return live.OrderResult{}, err
	}
	return live.OrderResult{}, fmt.Errorf("synthetic timeout after exchange acceptance")
}

func seedHermesOwnedPosition(t *testing.T, db *storage.DB, qty, price float64) {
	t.Helper()
	meta := live.OrderStatus{Source: "HERMES_OPERATOR"}
	if err := db.SaveManagedLiveOrder("hseedbtc01", "seed-order", "BTC-USDT", "BTCUSDT", "BUY", "limit", price, qty, price*qty, live.StatusFilled, meta); err != nil {
		t.Fatal(err)
	}
	e := live.LivePositionEvent{Timestamp: 1700000000, ClientOrderID: "hseedbtc01", OrderID: "seed-order", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "BUY", DeltaQuantity: qty, FillPrice: price, NotionalDelta: price * qty, Status: live.StatusFilled}
	if _, err := db.ApplyLivePositionEvent(e); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLivePositionEvent(e); err != nil {
		t.Fatal(err)
	}
}

func TestHermesSyntheticAcceptedTimeoutRecoveredByClientID(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "accepted-timeout.sqlite")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedHermesOwnedPosition(t, db, .01, 50000)
	owned, err := db.HermesOwnedPositions()
	if err != nil {
		t.Fatal(err)
	}
	fake := simulator.NewFakeOKX()
	fake.SetFilter("BTC-USDT", live.InstrumentFilter{Symbol: "BTCUSDT", InstID: "BTC-USDT", TickSize: .1, StepSize: .001, MinSize: .001, MinNotional: 1})
	result := lg.ExecuteHermesReduceActions(ctx, syntheticCfg(), "timeout-reduce", []lg.HermesActionDecision{{Allowed: true, Action: hermesoperator.Action{Symbol: "BTCUSDT", Intent: hermesoperator.IntentReduce, RequestedNotionalUSDT: 100, EntryPrice: 50000}}}, owned, []live.InstrumentFilter{{Symbol: "BTCUSDT", InstID: "BTC-USDT", TickSize: .1, StepSize: .001, MinSize: .001, MinNotional: 1}}, acceptedTimeoutPlacer{fake}, db, false)
	if len(result.Blocked) != 1 {
		t.Fatalf("accepted timeout should be recoverable block: %+v", result)
	}
	open, err := db.OpenLiveOrders()
	if err != nil || len(open) != 1 || open[0].Status != live.StatusPlanned {
		t.Fatalf("reservation not retained: %+v err=%v", open, err)
	}
	reconciled := lg.ReconcileOrders(ctx, lifecycleStatusReader{fake}, open)
	if reconciled.Unknown != 0 || len(reconciled.Orders) != 1 || reconciled.Orders[0].Status != live.StatusSubmitted {
		t.Fatalf("client-id recovery failed: %+v", reconciled)
	}
}

func TestHermesSyntheticSellPartialFillIsIdempotentAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sell-fill.sqlite")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedHermesOwnedPosition(t, db, .01, 50000)
	if err := db.SaveManagedLiveOrder("hexitbtc01", "exit-order", "BTC-USDT", "BTCUSDT", "SELL", "limit", 55000, .004, 220, live.StatusPartialFill, live.OrderStatus{Source: "HERMES_OPERATOR"}); err != nil {
		t.Fatal(err)
	}
	status := live.OrderStatus{ClientOrderID: "hexitbtc01", OrderID: "exit-order", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "SELL", Status: live.StatusPartialFill, AccumulatedFillSz: .002, AvgPrice: 55000}
	event, apply, err := lg.BuildPositionEvent(live.LiveFillSnapshot{}, status, time.Unix(1700000100, 0))
	if err != nil || !apply {
		t.Fatalf("sell event apply=%v err=%v", apply, err)
	}
	if _, err := db.ApplyLivePositionEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLivePositionEvent(event); err != nil {
		t.Fatal(err)
	}
	snap := lg.FillSnapshotFromStatus(status)
	if err := db.SaveLiveFillSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if _, apply, err := lg.BuildPositionEvent(snap, status, time.Unix(1700000200, 0)); err != nil || apply {
		t.Fatalf("duplicate fill reapplied apply=%v err=%v", apply, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owned, err := db.HermesOwnedPositions()
	if err != nil || len(owned) != 1 || math.Abs(owned[0].Quantity-.008) > 1e-12 {
		t.Fatalf("restart sell ownership wrong: %+v err=%v", owned, err)
	}
	positions, err := db.LivePositions()
	if err != nil || len(positions) != 1 || math.Abs(positions[0].Quantity-.008) > 1e-12 {
		t.Fatalf("restart position wrong: %+v err=%v", positions, err)
	}
}
