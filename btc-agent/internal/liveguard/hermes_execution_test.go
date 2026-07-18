package liveguard

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
)

func TestExecuteHermesDesiredOrdersBlocksWithoutAuthority(t *testing.T) {
	cfg := config.Config{}
	result := ExecuteHermesDesiredOrders(context.Background(), cfg, agent2.Plan{}, []ManagedDesiredOrder{{Source: "HERMES_OPERATOR", DecisionID: "d1", Intent: "PROBE_LIMIT"}}, nil, nil, nil, ManagedExecutionContext{}, false)
	if result.Status != ManagedCycleBlocked {
		t.Fatalf("expected blocked: %+v", result)
	}
}

func TestHermesClientOrderIDDeterministic(t *testing.T) {
	d := ManagedDesiredOrder{Symbol: "RENDERUSDT", LayerIndex: 1, DecisionID: "abc-def"}
	if a, b := hermesClientOrderID(d), hermesClientOrderID(d); a != b || a == "" {
		t.Fatalf("IDs not deterministic: %q %q", a, b)
	}
}

type hermesTestPlacer struct{ calls []live.LimitOrderRequest }

func (p *hermesTestPlacer) PlaceSpotLimitOrder(_ context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	p.calls = append(p.calls, req)
	return live.OrderResult{OrderID: "ord", InstID: req.InstID, ClientOrderID: req.ClientOrderID, Submitted: true}, nil
}

type hermesTestRecorder struct {
	reserved   []string
	submitted  []string
	reserveErr error
}

func (r *hermesTestRecorder) ReserveManagedLiveOrder(id string, _ ManagedDesiredOrder, _ string) error {
	if r.reserveErr != nil {
		return r.reserveErr
	}
	r.reserved = append(r.reserved, id)
	return nil
}
func (r *hermesTestRecorder) MarkManagedLiveOrderSubmitted(id string, _ live.OrderResult) error {
	r.submitted = append(r.submitted, id)
	return nil
}
func (r *hermesTestRecorder) MarkManagedLiveOrderRejected(string, string) error { return nil }
func (r *hermesTestRecorder) MarkManagedLiveOrderUnknown(string, string) error  { return nil }

func TestExecuteHermesCanaryProbeCallsSpotLimitOnce(t *testing.T) {
	cfg := config.Config{}
	cfg.HermesOperator.Enabled, cfg.HermesOperator.Mode = true, "canary"
	cfg.Live.Enabled, cfg.Live.AutoExecute = true, true
	cfg.Live.MaxLiveNotionalPerOrderUSDT, cfg.Live.MaxLiveNotionalPerAssetUSDT, cfg.Live.MaxLiveNotionalTotalUSDT = 10, 20, 20
	cfg.Live.FirstOrderRequireDryRun = false
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.NoFutures, cfg.Risk.NoLeverage, cfg.Risk.SpotLimitOnly = true, true, true
	d := ManagedDesiredOrder{Symbol: "RENDERUSDT", InstID: "RENDER-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 1.5, Quantity: 2, Notional: 3, PostOnly: true, Source: "HERMES_OPERATOR", DecisionID: "decision-1", Intent: "PROBE_LIMIT", AllocationTier: string(OpportunityProbe)}
	placer, recorder := &hermesTestPlacer{}, &hermesTestRecorder{}
	result := ExecuteHermesDesiredOrders(context.Background(), cfg, agent2.Plan{State: agent2.StateScout}, []ManagedDesiredOrder{d}, nil, placer, recorder, ManagedExecutionContext{HermesMode: "canary"}, false)
	if result.Status != ManagedCycleCompleted || len(placer.calls) != 1 || len(recorder.reserved) != 1 {
		t.Fatalf("result=%+v calls=%d reserved=%d", result, len(placer.calls), len(recorder.reserved))
	}
	if placer.calls[0].Side != "buy" || !placer.calls[0].PostOnly {
		t.Fatalf("unsafe request: %+v", placer.calls[0])
	}
}

func TestExecuteHermesCanaryDuplicateReservationDoesNotCallExchange(t *testing.T) {
	cfg := config.Config{}
	cfg.HermesOperator.Enabled, cfg.HermesOperator.Mode = true, "canary"
	cfg.Live.Enabled, cfg.Live.AutoExecute = true, true
	cfg.Live.MaxLiveNotionalPerOrderUSDT, cfg.Live.MaxLiveNotionalPerAssetUSDT, cfg.Live.MaxLiveNotionalTotalUSDT = 10, 20, 20
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.NoFutures, cfg.Risk.NoLeverage, cfg.Risk.SpotLimitOnly = true, true, true
	d := ManagedDesiredOrder{Symbol: "RENDERUSDT", InstID: "RENDER-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 1.5, Quantity: 2, Notional: 3, PostOnly: true, Source: "HERMES_OPERATOR", DecisionID: "decision-1", Intent: "PROBE_LIMIT", AllocationTier: string(OpportunityProbe)}
	placer, recorder := &hermesTestPlacer{}, &hermesTestRecorder{reserveErr: fmt.Errorf("UNIQUE constraint")}
	result := ExecuteHermesDesiredOrders(context.Background(), cfg, agent2.Plan{State: agent2.StateScout}, []ManagedDesiredOrder{d}, nil, placer, recorder, ManagedExecutionContext{HermesMode: "canary"}, false)
	if len(placer.calls) != 0 || len(result.Blocked) != 1 {
		t.Fatalf("duplicate reached exchange: %+v", result)
	}
}

type hermesCancelFixture struct {
	cancelCalls     []live.CancelOrderRequest
	cancelErr       error
	cancelConfirmed bool
	remote          live.OrderStatus
	statusErr       error
	saved           []live.OrderStatus
	events          []live.OrderStatus
}

func (f *hermesCancelFixture) CancelOrder(_ context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	f.cancelCalls = append(f.cancelCalls, req)
	return live.CancelOrderResult{InstID: req.InstID, OrderID: req.OrderID, ClientOrderID: req.ClientOrderID, Canceled: f.cancelConfirmed}, f.cancelErr
}

func (f *hermesCancelFixture) OrderStatus(_ context.Context, _, _, _ string) (live.OrderStatus, error) {
	if f.statusErr != nil {
		return live.OrderStatus{}, f.statusErr
	}
	return f.remote, nil
}
func (f *hermesCancelFixture) PendingOrders(context.Context, string) ([]live.OrderStatus, error) {
	return nil, nil
}

func (f *hermesCancelFixture) SaveLiveOrderStatus(o live.OrderStatus) error {
	f.saved = append(f.saved, o)
	return nil
}
func (f *hermesCancelFixture) SaveLiveOrderEvent(o live.OrderStatus) error {
	f.events = append(f.events, o)
	return nil
}
func cancelCfg() config.Config {
	c := config.Config{}
	c.HermesOperator.Enabled = true
	c.HermesOperator.Mode = "canary"
	c.Live.Enabled = true
	c.Live.AutoExecute = true
	c.Execution.RealTradingEnabled = true
	return c
}
func cancelDecision(symbol string) HermesActionDecision {
	return HermesActionDecision{Allowed: true, Action: hermesoperator.Action{Symbol: symbol, Intent: hermesoperator.IntentCancel}}
}

func TestExecuteHermesCancelBlocksUnownedAndAmbiguousOrders(t *testing.T) {
	f := &hermesCancelFixture{cancelConfirmed: true}
	unowned := live.OrderStatus{Symbol: "BTCUSDT", InstID: "BTC-USDT", ClientOrderID: "agent2-1", OrderID: "o1", Source: "deterministic_agent2_layer_1", Status: live.StatusSubmitted}
	got := ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{unowned}, f, f, f, false)
	if len(f.cancelCalls) != 0 || len(got.Blocked) != 1 {
		t.Fatalf("unowned reached canceler: %+v", got)
	}
	owned1 := unowned
	owned1.ClientOrderID = "hdecisionbtc01"
	owned1.Source = "HERMES_OPERATOR"
	owned2 := owned1
	owned2.ClientOrderID = "hdecisionbtc02"
	owned2.OrderID = "o2"
	got = ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{owned1, owned2}, f, f, f, false)
	if len(f.cancelCalls) != 0 || len(got.Blocked) != 1 {
		t.Fatalf("ambiguous reached canceler: %+v", got)
	}
}

func TestExecuteHermesCancelPersistsOnlyConfirmedOutcome(t *testing.T) {
	order := live.OrderStatus{Symbol: "BTCUSDT", InstID: "BTC-USDT", ClientOrderID: "hdecisionbtc01", OrderID: "o1", Source: "HERMES_OPERATOR", Status: live.StatusSubmitted}
	unknown := &hermesCancelFixture{cancelErr: fmt.Errorf("timeout")}
	got := ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{order}, unknown, unknown, unknown, false)
	if len(unknown.cancelCalls) != 1 || len(unknown.saved) != 0 || len(got.Blocked) != 1 {
		t.Fatalf("unknown outcome persisted: %+v", got)
	}
	confirmed := &hermesCancelFixture{cancelConfirmed: true, remote: live.OrderStatus{Status: live.StatusCancelled}}
	got = ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{order}, confirmed, confirmed, confirmed, false)
	if len(got.Canceled) != 1 || len(confirmed.saved) != 1 || len(confirmed.events) != 1 || confirmed.saved[0].Status != live.StatusCancelled {
		t.Fatalf("confirmed cancellation not persisted: %+v", got)
	}
}

func TestExecuteHermesCancelDryRunNeverCallsExchange(t *testing.T) {
	order := live.OrderStatus{Symbol: "BTCUSDT", InstID: "BTC-USDT", ClientOrderID: "hdecisionbtc01", OrderID: "o1", Source: "HERMES_OPERATOR", Status: live.StatusSubmitted}
	f := &hermesCancelFixture{cancelConfirmed: true}
	got := ExecuteHermesCancelActions(context.Background(), config.Config{}, "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{order}, f, f, f, true)
	if got.Status != ManagedCycleDryRun || len(got.Canceled) != 1 || len(f.cancelCalls) != 0 {
		t.Fatalf("dry-run called exchange: %+v", got)
	}
}

func TestExecuteHermesCancelPreservesPartialFillForLedgerReconcile(t *testing.T) {
	order := live.OrderStatus{Symbol: "BTCUSDT", InstID: "BTC-USDT", ClientOrderID: "hdecisionbtc01", OrderID: "o1", Source: "HERMES_OPERATOR", Status: live.StatusSubmitted}
	f := &hermesCancelFixture{cancelConfirmed: true, remote: live.OrderStatus{Status: live.StatusCancelled, AccumulatedFillSz: 0.002, AvgPrice: 60000}}
	got := ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{order}, f, f, f, false)
	if len(got.Canceled) != 1 || len(f.saved) != 1 || f.saved[0].Status != live.StatusPartialFill || f.saved[0].AccumulatedFillSz != 0.002 {
		t.Fatalf("partial fill lost after cancel: %+v saved=%+v", got, f.saved)
	}
}

func TestExecuteHermesCancelPostCancelStatusErrorRemainsOpen(t *testing.T) {
	order := live.OrderStatus{Symbol: "BTCUSDT", InstID: "BTC-USDT", ClientOrderID: "hdecisionbtc01", OrderID: "o1", Source: "HERMES_OPERATOR", Status: live.StatusSubmitted}
	f := &hermesCancelFixture{cancelConfirmed: true, statusErr: fmt.Errorf("status timeout")}
	got := ExecuteHermesCancelActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{cancelDecision("BTCUSDT")}, []live.OrderStatus{order}, f, f, f, false)
	if len(got.Blocked) != 1 || len(f.saved) != 0 || len(f.events) != 0 {
		t.Fatalf("unknown post-cancel status persisted: %+v", got)
	}
}

func reduceDecision(symbol string, notional, price float64) HermesActionDecision {
	return HermesActionDecision{Allowed: true, Action: hermesoperator.Action{Symbol: symbol, Intent: hermesoperator.IntentReduce, RequestedNotionalUSDT: notional, EntryPrice: price}}
}
func reduceFilter() []live.InstrumentFilter {
	return []live.InstrumentFilter{{Symbol: "BTCUSDT", InstID: "BTC-USDT", TickSize: 0.1, StepSize: 0.001, MinSize: 0.001, MinNotional: 1}}
}

func TestExecuteHermesReduceBlocksWithoutOwnedPosition(t *testing.T) {
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	got := ExecuteHermesReduceActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{reduceDecision("BTCUSDT", 100, 50000)}, nil, reduceFilter(), p, r, false)
	if len(p.calls) != 0 || len(got.Blocked) != 1 {
		t.Fatalf("unowned REDUCE reached exchange: %+v", got)
	}
}

func TestExecuteHermesReduceCapsQuantityAndPlacesSell(t *testing.T) {
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: 0.0015, CostBasis: 60}}
	got := ExecuteHermesReduceActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{reduceDecision("BTCUSDT", 100, 50000)}, owned, reduceFilter(), p, r, false)
	if len(got.Placed) != 1 || len(p.calls) != 1 || p.calls[0].Side != "sell" || p.calls[0].Quantity != 0.001 || p.calls[0].PostOnly {
		t.Fatalf("unsafe REDUCE: %+v calls=%+v", got, p.calls)
	}
}

func TestExecuteHermesReduceDryRunDoesNotPlace(t *testing.T) {
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: 0.01, CostBasis: 500}}
	got := ExecuteHermesReduceActions(context.Background(), config.Config{}, "d1", []HermesActionDecision{reduceDecision("BTCUSDT", 100, 50000)}, owned, reduceFilter(), p, r, true)
	if got.Status != ManagedCycleDryRun || len(got.Placed) != 1 || len(p.calls) != 0 || len(r.reserved) != 0 {
		t.Fatalf("REDUCE dry-run touched exchange/storage: %+v", got)
	}
}

type reduceErrorPlacer struct{}

func (*reduceErrorPlacer) PlaceSpotLimitOrder(context.Context, live.LimitOrderRequest) (live.OrderResult, error) {
	return live.OrderResult{}, fmt.Errorf("timeout")
}
func TestExecuteHermesReduceUnknownPlacementKeepsReservationUnrejected(t *testing.T) {
	r := &hermesTestRecorder{}
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: 0.01, CostBasis: 500}}
	got := ExecuteHermesReduceActions(context.Background(), cancelCfg(), "d1", []HermesActionDecision{reduceDecision("BTCUSDT", 100, 50000)}, owned, reduceFilter(), &reduceErrorPlacer{}, r, false)
	if len(got.Blocked) != 1 || len(r.reserved) != 1 || len(r.submitted) != 0 {
		t.Fatalf("unknown REDUCE outcome lifecycle wrong: %+v", got)
	}
}

func exitLimitDecision(symbol string, notional, price float64) HermesActionDecision {
	return HermesActionDecision{Allowed: true, Action: hermesoperator.Action{Symbol: symbol, Intent: hermesoperator.IntentExitLimit, RequestedNotionalUSDT: notional, EntryPrice: price}}
}
func TestExecuteHermesExitLimitUsesOwnedSellLifecycle(t *testing.T) {
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: 0.003, CostBasis: 150}}
	got := ExecuteHermesExitLimitActions(context.Background(), cancelCfg(), "d-exit", []HermesActionDecision{exitLimitDecision("BTCUSDT", 1000, 50000)}, owned, reduceFilter(), p, r, false)
	if len(got.Placed) != 1 || len(p.calls) != 1 || p.calls[0].Side != "sell" || p.calls[0].Quantity != 0.003 || got.Placed[0].Desired.Intent != "EXIT_LIMIT" {
		t.Fatalf("unsafe EXIT_LIMIT: %+v calls=%+v", got, p.calls)
	}
}
func TestExecuteHermesExitLimitBlocksUnownedPosition(t *testing.T) {
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	got := ExecuteHermesExitLimitActions(context.Background(), cancelCfg(), "d-exit", []HermesActionDecision{exitLimitDecision("BTCUSDT", 10, 50000)}, nil, reduceFilter(), p, r, false)
	if len(got.Blocked) != 1 || len(p.calls) != 0 || len(r.reserved) != 0 {
		t.Fatalf("unowned EXIT_LIMIT reached execution: %+v", got)
	}
}

func TestHermesExitSubtractsOpenSellResidual(t *testing.T) {
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: .01, AvgEntryPrice: 50000}}
	open := []live.OrderStatus{{Symbol: "BTCUSDT", Side: "SELL", Status: live.StatusPartialFill, Quantity: .008, FilledQuantity: .003}}
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	got := ExecuteHermesExitLimitActionsWithOpen(context.Background(), cancelCfg(), "d-residual", []HermesActionDecision{exitLimitDecision("BTCUSDT", 1000, 50000)}, owned, open, reduceFilter(), p, r, false)
	if len(got.Placed) != 1 || len(p.calls) != 1 {
		t.Fatalf("expected one residual-capped exit, got %+v requests=%+v", got, p.calls)
	}
	if math.Abs(p.calls[0].Quantity-.005) > 1e-12 {
		t.Fatalf("quantity=%v want unreserved residual .005", p.calls[0].Quantity)
	}
}

func TestHermesExitBlocksWhenOpenSellReservesOwnership(t *testing.T) {
	owned := []live.LivePosition{{Symbol: "BTCUSDT", InstID: "BTC-USDT", Quantity: .01, AvgEntryPrice: 50000}}
	open := []live.OrderStatus{{Symbol: "BTCUSDT", Side: "SELL", Status: live.StatusSubmitted, Quantity: .01}}
	p, r := &hermesTestPlacer{}, &hermesTestRecorder{}
	got := ExecuteHermesExitLimitActionsWithOpen(context.Background(), cancelCfg(), "d-reserved", []HermesActionDecision{exitLimitDecision("BTCUSDT", 1000, 50000)}, owned, open, reduceFilter(), p, r, false)
	if len(got.Placed) != 0 || len(got.Blocked) != 1 || len(p.calls) != 0 {
		t.Fatalf("fully reserved ownership must block overlapping exit: %+v requests=%+v", got, p.calls)
	}
}

func TestOpenSellResidualIgnoresTerminalAndBuyOrders(t *testing.T) {
	orders := []live.OrderStatus{
		{Symbol: "BTCUSDT", Side: "SELL", Status: live.StatusFilled, Quantity: .4, FilledQuantity: .4},
		{Symbol: "BTCUSDT", Side: "BUY", Status: live.StatusSubmitted, Quantity: .3},
		{Symbol: "BTCUSDT", Side: "SELL", Status: live.StatusPartialFill, Quantity: .2, AccumulatedFillSz: .075},
	}
	if got := openSellResidualQuantity("BTCUSDT", orders); math.Abs(got-.125) > 1e-12 {
		t.Fatalf("residual=%v want .125", got)
	}
}

func TestHermesOwnedSellBlocksBelowAverageEntry(t *testing.T) {
	cfg := cancelCfg()
	p := &hermesTestPlacer{}
	r := &hermesTestRecorder{}
	owned := []live.LivePosition{{Symbol: "RENDERUSDT", InstID: "RENDER-USDT", Quantity: 4, AvgEntryPrice: 2}}
	filters := []live.InstrumentFilter{{Symbol: "RENDERUSDT", InstID: "RENDER-USDT", TickSize: .01, StepSize: .1, MinSize: .1, MinNotional: 1}}
	decision := HermesActionDecision{Allowed: true, Action: hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentExitLimit, EntryPrice: 1.5, RequestedNotionalUSDT: 3}}
	got := ExecuteHermesExitLimitActionsWithOpen(context.Background(), cfg, "loss-sale", []HermesActionDecision{decision}, owned, nil, filters, p, r, false)
	if len(got.Placed) != 0 || len(p.calls) != 0 || len(got.Blocked) != 1 || !strings.Contains(got.Blocked[0].Reason, "automated loss sale forbidden") {
		t.Fatalf("loss sale reached execution: %+v calls=%+v", got, p.calls)
	}
}
