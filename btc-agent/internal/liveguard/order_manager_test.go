package liveguard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

type fakeManagedExchange struct {
	placed    []live.LimitOrderRequest
	canceled  []live.CancelOrderRequest
	placeErr  error
	cancelErr error
	status    live.OrderStatus
	statusErr error
	book      liquidity.OrderBookSnapshot
	bookErr   error
}

func (f *fakeManagedExchange) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.placed = append(f.placed, req)
	if f.placeErr != nil {
		return live.OrderResult{InstID: req.InstID, ClientOrderID: req.ClientOrderID}, f.placeErr
	}
	return live.OrderResult{InstID: req.InstID, OrderID: "ord", ClientOrderID: req.ClientOrderID, Submitted: true}, nil
}

func (f *fakeManagedExchange) CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	f.canceled = append(f.canceled, req)
	if f.cancelErr != nil {
		return live.CancelOrderResult{InstID: req.InstID, OrderID: req.OrderID, ClientOrderID: req.ClientOrderID}, f.cancelErr
	}
	return live.CancelOrderResult{InstID: req.InstID, OrderID: req.OrderID, ClientOrderID: req.ClientOrderID, Canceled: true, Code: "0"}, nil
}

func (f *fakeManagedExchange) OrderStatus(_ context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
	if f.statusErr != nil {
		return live.OrderStatus{}, f.statusErr
	}
	if f.status.Status == "" {
		return live.OrderStatus{InstID: instID, OrderID: orderID, ClientOrderID: clientOrderID, Status: live.StatusCancelled}, nil
	}
	return f.status, nil
}

func (f *fakeManagedExchange) PendingOrders(context.Context, string) ([]live.OrderStatus, error) {
	return nil, nil
}

func (f *fakeManagedExchange) OrderBook(ctx context.Context, instID string) (liquidity.OrderBookSnapshot, error) {
	if f.bookErr != nil {
		return liquidity.OrderBookSnapshot{}, f.bookErr
	}
	if f.book.BestBid == 0 && f.book.BestAsk == 0 {
		return liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}, nil
	}
	return f.book, nil
}

func confirmedExecContext() ManagedExecutionContext {
	return ManagedExecutionContext{BTCAccumulationPhase: "ACCUMULATION_CONFIRMED", FirstOrderDryRunApproved: true}
}

func manageLiveOrdersConfirmed(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader) ManagedCycleResult {
	return ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, confirmedExecContext(), nil, false)
}

func manageLiveOrdersWithRecorderConfirmed(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, recorder ManagedOrderRecorder) ManagedCycleResult {
	return ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, confirmedExecContext(), recorder, false)
}

type fakeManagedRecorder struct {
	reserveErr   error
	reserved     []string
	submitted    []string
	rejected     []string
	rejectReason []string
	unknown      []string
	unknownErr   error
}

func (f *fakeManagedRecorder) ReserveManagedLiveOrder(clientOrderID string, desired ManagedDesiredOrder, reason string) error {
	if f.reserveErr != nil {
		return f.reserveErr
	}
	f.reserved = append(f.reserved, clientOrderID)
	return nil
}

func (f *fakeManagedRecorder) MarkManagedLiveOrderSubmitted(clientOrderID string, result live.OrderResult) error {
	f.submitted = append(f.submitted, clientOrderID)
	return nil
}

func (f *fakeManagedRecorder) MarkManagedLiveOrderRejected(clientOrderID string, reason string) error {
	f.rejected = append(f.rejected, clientOrderID)
	f.rejectReason = append(f.rejectReason, reason)
	return nil
}

func (f *fakeManagedRecorder) MarkManagedLiveOrderUnknown(clientOrderID string, reason string) error {
	if f.unknownErr != nil {
		return f.unknownErr
	}
	f.unknown = append(f.unknown, clientOrderID)
	return nil
}

func TestManageLiveOrdersAllowsMultipleAssetsAndLayers(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if got.Status != ManagedCycleCompleted {
		t.Fatalf("status=%s summary=%s", got.Status, got.Summary)
	}
	if len(got.Desired) != 4 || len(got.Placed) != 4 || len(ex.placed) != 4 {
		t.Fatalf("desired=%d placed=%d exchange=%d result=%+v", len(got.Desired), len(got.Placed), len(ex.placed), got)
	}
}

func TestManageLiveOrdersCancelsWhenPlanNotActive(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	plan.State = agent2.StateWatch
	ex := &fakeManagedExchange{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Canceled) != 1 || len(ex.canceled) != 1 {
		t.Fatalf("expected cancel, got %+v", got)
	}
}

func TestManageLiveOrdersKeepsMatchingOrder(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	ex := &fakeManagedExchange{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Kept) != 1 || len(got.Canceled) != 0 || len(ex.canceled) != 0 {
		t.Fatalf("expected keep, got %+v", got)
	}
}

func TestManageLiveOrdersCancelsWhenPlanArmed(t *testing.T) {
	cfg := managedConfig()
	plan := agent2.Plan{State: agent2.StateArmed, ActionPermission: agent1.Armed, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateArmed, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 88, Reason: "armed", Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 10}}}}}
	ex := &fakeManagedExchange{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Desired) != 0 || len(got.Placed) != 0 || len(got.Kept) != 0 || len(got.Canceled) != 1 || len(ex.canceled) != 1 {
		t.Fatalf("ARMED should cancel stale live order and not keep/place: %+v", got)
	}
}

func managedConfig() config.Config {
	var cfg config.Config
	cfg.Live.Enabled = true
	cfg.Live.AutoExecute = true
	cfg.Live.OrderManagementEnabled = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2
	cfg.Live.MaxOrderNotionalUSDT = 10
	cfg.Live.RequirePostOnly = true
	cfg.Live.MaxAutoLayersPerAsset = 2
	cfg.Live.MaxOpenLiveOrdersPerAsset = 2
	cfg.Live.MaxOpenLiveOrdersTotal = 6
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 2
	cfg.Live.MaxLiveNotionalPerAssetUSDT = 4
	cfg.Live.MaxLiveNotionalTotalUSDT = 12
	cfg.Live.CancelIfPlanNotActive = true
	cfg.Live.ReplaceIfPriceDriftPct = 0.01
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.35, "SOLUSDT": 0.45}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.70
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	return cfg
}

func managedPlan() agent2.Plan {
	return agent2.Plan{State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed, Assets: []agent2.AssetPlan{
		{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 88, Reason: "eth active", Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 10}, {Index: 2, Price: 95, Notional: 10}}},
		{Symbol: "SOLUSDT", State: agent2.StateActiveLimit, DiscountZone: market.Zone{Low: 45, High: 50}, Invalidation: 44, Reason: "sol active", Layers: []agent2.Layer{{Index: 1, Price: 50, Notional: 10}, {Index: 2, Price: 47, Notional: 10}}},
	}}
}

func TestManageLiveOrdersWithRecorderReservesBeforeSubmit(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	rec := &fakeManagedRecorder{}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if got.Status != ManagedCycleCompleted {
		t.Fatalf("status=%s summary=%s", got.Status, got.Summary)
	}
	if len(rec.reserved) != 4 || len(rec.submitted) != 4 || len(ex.placed) != 4 {
		t.Fatalf("reserve=%d submitted=%d placed=%d result=%+v", len(rec.reserved), len(rec.submitted), len(ex.placed), got)
	}
}

func TestManageLiveOrdersWithRecorderBlocksWhenReserveFails(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	rec := &fakeManagedRecorder{reserveErr: fmt.Errorf("duplicate client id")}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if got.Status != ManagedCyclePartial || len(ex.placed) != 0 || len(got.Blocked) == 0 {
		t.Fatalf("reserve failure should block before exchange call: placed=%d result=%+v", len(ex.placed), got)
	}
}

func TestManageLiveOrdersWithRecorderMarksRejectedOnSubmitError(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{placeErr: fmt.Errorf("exchange rejected api_secret=bad")}
	rec := &fakeManagedRecorder{}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if got.Status != ManagedCyclePartial || len(rec.rejected) == 0 || len(got.Blocked) == 0 {
		t.Fatalf("submit error should mark rejected: %+v recorder=%+v", got, rec)
	}
}

func TestManageLiveOrdersUnknownPersistenceFailureFailsClosed(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{placeErr: context.DeadlineExceeded}
	rec := &fakeManagedRecorder{unknownErr: fmt.Errorf("sqlite unavailable")}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	joined := strings.Join(append(got.Reasons, got.Blocked[0].Error), " ")
	if got.Status != ManagedCyclePartial || len(rec.rejected) != 0 || !strings.Contains(joined, "persist outcome failed") || !strings.Contains(got.Blocked[0].Reason, "manual check") {
		t.Fatalf("unknown persistence failure must fail closed without rejection: result=%+v recorder=%+v", got, rec)
	}
}

func TestManageLiveOrdersBlocksNilExchangeDependencies(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, nil, nil, fakeHaltReader{halted: false})
	if got.Status != ManagedCycleBlocked || !strings.Contains(strings.Join(got.Reasons, " "), "order placer/canceler unavailable") {
		t.Fatalf("nil exchange dependencies must fail closed: %+v", got)
	}
}

func TestManageLiveOrdersPostCancelStatusErrorBlocksReplacement(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{statusErr: context.DeadlineExceeded}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusSubmitted, Price: 80, Quantity: 0.025, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(ex.canceled) != 1 || len(got.Canceled) != 0 || len(got.Replaced) != 0 || len(ex.placed) != 3 || len(got.Blocked) == 0 {
		t.Fatalf("unknown post-cancel status must keep original slot and block replacement: %+v placed=%d", got, len(ex.placed))
	}
}

func TestManageLiveOrdersCancelFillRaceBlocksReplacement(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{status: live.OrderStatus{Status: live.StatusCancelled, AccumulatedFillSz: 0.01, AvgPrice: 80}}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusSubmitted, Price: 80, Quantity: 0.025, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Canceled) != 0 || len(got.Replaced) != 0 || len(ex.placed) != 3 || len(got.Blocked) == 0 || live.NormalizeOrderStatus(got.Blocked[0].Order.Status) != live.StatusPartialFill {
		t.Fatalf("cancel/fill race must remain partial and block replacement: %+v placed=%d", got, len(ex.placed))
	}
}

func TestManageLiveOrdersTerminalCancelAllowsReplacement(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{status: live.OrderStatus{Status: live.StatusCancelled}}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusSubmitted, Price: 80, Quantity: 0.025, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Canceled) != 1 || len(got.Replaced) != 1 || len(ex.placed) != 4 || live.NormalizeOrderStatus(got.Canceled[0].Order.Status) != live.StatusCancelled {
		t.Fatalf("confirmed zero-fill cancellation should permit replacement: %+v placed=%d", got, len(ex.placed))
	}
}

func TestManageLiveOrdersCancelErrorIsPartialAndBlocked(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	plan.State = agent2.StateWatch
	ex := &fakeManagedExchange{cancelErr: fmt.Errorf("cancel rejected")}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if got.Status != ManagedCyclePartial || len(got.Canceled) != 0 || len(got.Blocked) != 1 || len(ex.canceled) != 1 {
		t.Fatalf("cancel error must be partial and blocked: %+v", got)
	}
}

func TestManageLiveOrdersTotalCapBlocksBeforeExchangeCall(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.MaxOpenLiveOrdersTotal = 1
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(ex.placed) != 0 || len(got.Blocked) == 0 {
		t.Fatalf("total open-order cap must block before exchange call: placed=%d result=%+v", len(ex.placed), got)
	}
	foundCapBlock := false
	for _, blocked := range got.Blocked {
		if blocked.Reason == "total open order limit reached" {
			foundCapBlock = true
		}
	}
	if !foundCapBlock {
		t.Fatalf("missing total open-order cap block: %+v", got.Blocked)
	}
}

func TestManageLiveOrdersDryRunDoesNotCallExchangeWhenHalted(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	got := ManageLiveOrdersDryRun(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: true}, true)
	if got.Status != ManagedCycleDryRun {
		t.Fatalf("status=%s summary=%s", got.Status, got.Summary)
	}
	if len(got.Placed) != 4 || len(ex.placed) != 0 || len(ex.canceled) != 0 {
		t.Fatalf("dry-run should simulate without exchange calls: placed=%d exch_place=%d exch_cancel=%d", len(got.Placed), len(ex.placed), len(ex.canceled))
	}
}

func TestManageLiveOrdersBuildsPerCoinSummaries(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false})
	eth := managedCoinForTest(t, got.PerCoin, "ETHUSDT")
	sol := managedCoinForTest(t, got.PerCoin, "SOLUSDT")
	if eth.State != agent2.StateActiveLimit || eth.DesiredLayers != 2 || eth.Placed != 2 || eth.PendingNotional != 4 {
		t.Fatalf("bad ETH summary: %+v", eth)
	}
	if sol.State != agent2.StateActiveLimit || sol.DesiredLayers != 2 || sol.Placed != 2 || sol.PendingNotional != 4 {
		t.Fatalf("bad SOL summary: %+v", sol)
	}
}

func TestManageLiveOrdersPerCoinIncludesIdleConfiguredAsset(t *testing.T) {
	cfg := managedConfig()
	cfg.Data.Symbols.Assets = append(cfg.Data.Symbols.Assets, "RENDERUSDT")
	plan := managedPlan()
	got := ManageLiveOrdersDryRun(context.Background(), cfg, plan, nil, nil, nil, nil, nil, fakeHaltReader{halted: false}, true)
	render := managedCoinForTest(t, got.PerCoin, "RENDERUSDT")
	if render.State != agent2.StateNoTrade || render.DesiredLayers != 0 || render.Placed != 0 || len(render.Actions) != 0 {
		t.Fatalf("bad idle summary: %+v", render)
	}
}

func TestManageLiveOrdersPerCoinAssignsCancelAndReplace(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	open := []live.OrderStatus{
		{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 80, Quantity: 0.025, Notional: 2, LayerIndex: 1},
		{InstID: "SOL-USDT", Symbol: "SOLUSDT", ClientOrderID: "c2", OrderID: "o2", Status: live.StatusLiveOpen, Price: 10, Quantity: 0.2, Notional: 2, LayerIndex: 9},
	}
	got := ManageLiveOrdersDryRun(context.Background(), cfg, plan, open, nil, nil, nil, nil, fakeHaltReader{halted: false}, true)
	eth := managedCoinForTest(t, got.PerCoin, "ETHUSDT")
	sol := managedCoinForTest(t, got.PerCoin, "SOLUSDT")
	if eth.Replaced != 1 {
		t.Fatalf("expected ETH replace summary, got %+v", eth)
	}
	if sol.Canceled != 1 {
		t.Fatalf("expected SOL cancel summary, got %+v", sol)
	}
}

func TestManageLiveOrdersMMGateBlocksBeforeReserveAndSubmit(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.LiquidityGateEnabled = true
	cfg.Live.RequireOrderBookLiquidity = true
	cfg.Live.MaxSpreadBps = 15
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{book: liquidity.OrderBookSnapshot{BestBid: 99, BestAsk: 101, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}}
	rec := &fakeManagedRecorder{}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if len(got.Placed) != 0 || len(ex.placed) != 0 || len(rec.reserved) != 0 || len(got.Blocked) == 0 {
		t.Fatalf("MM gate should block before reserve/submit: placed=%d exch=%d reserved=%d result=%+v", len(got.Placed), len(ex.placed), len(rec.reserved), got)
	}
	found := false
	for _, b := range got.Blocked {
		if strings.Contains(b.Reason, "MM execution gate blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing MM gate block: %+v", got.Blocked)
	}
}

func TestManageLiveOrdersMMGateHealthyBookAllowsSubmit(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.LiquidityGateEnabled = true
	cfg.Live.RequireOrderBookLiquidity = true
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{book: liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}}
	rec := &fakeManagedRecorder{}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if len(got.Placed) == 0 || len(ex.placed) == 0 || len(rec.reserved) == 0 {
		t.Fatalf("healthy MM gate should allow submit: placed=%d exch=%d reserved=%d result=%+v", len(got.Placed), len(ex.placed), len(rec.reserved), got)
	}
}

func TestBuildManagedDesiredOrdersBlocksLiquidityFail(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.LiquidityGateEnabled = true
	plan := managedPlan()
	plan.Assets[0].LiquidityQuality = liquidity.Quality{Enabled: true, Pass: false, Grade: liquidity.GradeD, Reasons: []string{"liquidity gate: test fail"}}
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	for _, d := range desired {
		if d.Symbol == "ETHUSDT" {
			t.Fatalf("ETH should be blocked by liquidity: desired=%+v blocked=%+v", desired, blocked)
		}
	}
	found := false
	for _, b := range blocked {
		if b.Symbol == "ETHUSDT" && strings.Contains(b.Reason, "liquidity gate blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing liquidity block: desired=%+v blocked=%+v", desired, blocked)
	}
}

func TestAssertManagedExecutionAllowedBlocksUnsafeOrder(t *testing.T) {
	cfg := managedConfig()
	cfg.Execution.RealTradingEnabled = true
	plan := managedPlan()
	d := ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1, Side: "SELL", Type: "market", Price: 100, Quantity: 1, Notional: 2, PostOnly: false}
	blockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: d, DryRun: true, ManagedExecutionContext: confirmedExecContext()})
	joined := strings.Join(blockers, ";")
	for _, want := range []string{"desired side must be BUY", "desired type must be limit", "desired order must be post-only"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in blockers %v", want, blockers)
		}
	}
}

func TestAssertManagedExecutionAllowedPassesValidDryRun(t *testing.T) {
	cfg := managedConfig()
	cfg.Execution.RealTradingEnabled = true
	plan := managedPlan()
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	blockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: desired[0], DryRun: true, ManagedExecutionContext: confirmedExecContext()})
	if len(blockers) != 0 {
		t.Fatalf("valid dry-run assertion should pass: %v", blockers)
	}
}

func TestManageLiveOrdersFinalAssertionBlocksBeforeSubmit(t *testing.T) {
	cfg := managedConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.SpotLimitOnly = false
	plan := managedPlan()
	ex := &fakeManagedExchange{}
	rec := &fakeManagedRecorder{}
	got := manageLiveOrdersWithRecorderConfirmed(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, rec)
	if len(ex.placed) != 0 || len(rec.submitted) != 0 || len(got.Blocked) == 0 {
		t.Fatalf("final assertion should block before exchange submit: placed=%d submitted=%d result=%+v", len(ex.placed), len(rec.submitted), got)
	}
	found := false
	for _, b := range got.Blocked {
		if strings.Contains(b.Reason, "final execution assertion blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing final assertion block: %+v", got.Blocked)
	}
}

func TestFirstOrderQuarantineLimitsDesiredOrders(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.FirstOrderQuarantineEnabled = true
	cfg.Live.FirstOrderMaxNotionalUSDT = 1
	plan := managedPlan()
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(desired) != 1 || len(blocked) == 0 {
		t.Fatalf("quarantine should keep one desired and block rest: desired=%d blocked=%d", len(desired), len(blocked))
	}
	if desired[0].Notional > 1+1e-9 {
		t.Fatalf("quarantine cap not applied: %+v", desired[0])
	}
}

func TestForcedActiveLimitSimulationDryRun(t *testing.T) {
	cfg := managedConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Live.FirstOrderQuarantineEnabled = true
	cfg.Live.FirstOrderMaxNotionalUSDT = 1
	got := RunForcedActiveLimitSimulation(cfg)
	if !got.Passed || got.Desired == 0 || got.WouldPlace == 0 {
		t.Fatalf("forced simulation should pass: %+v", got)
	}
}

func TestBuildManagedDesiredOrdersSortsByHistoryQuality(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 20, Grade: "C"}, "SOLUSDT": {Score: 80, Grade: "A"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	if desired[0].Symbol != "SOLUSDT" || desired[0].QualityScore != 80 || desired[0].QualityGrade != "A" {
		t.Fatalf("quality priority missing: %+v", desired)
	}
}

func TestBuildManagedDesiredOrdersSkipsDGradeInLiveAuto(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 57, Grade: "B"}, "SOLUSDT": {Score: 0, Grade: "D"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	for _, d := range desired {
		if d.Symbol == "SOLUSDT" {
			t.Fatalf("D-grade SOL should be skipped in live auto: %+v", desired)
		}
	}
	foundBlock := false
	for _, b := range blocked {
		if b.Symbol == "SOLUSDT" && b.Reason == "live quality filter blocked D-grade coin" {
			foundBlock = true
		}
	}
	if !foundBlock {
		t.Fatalf("missing SOL D-grade block: %+v", blocked)
	}
}

func TestAllocateLiveCapitalQualityMultipliers(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	quality := map[string]historyQualityScore{"ETHUSDT": {Score: 70, Grade: "C"}, "SOLUSDT": {Score: 0, Grade: "NO_SAMPLE"}}
	alloc := AllocateLiveCapital(cfg, plan, quality, nil)
	if alloc["ETHUSDT"].QualityMultiplier != 0.5 || alloc["ETHUSDT"].MaxLayers > 2 {
		t.Fatalf("C quality should reduce size: %+v", alloc["ETHUSDT"])
	}
	if alloc["SOLUSDT"].QualityMultiplier != 0.25 || alloc["SOLUSDT"].MaxLayers != 1 || alloc["SOLUSDT"].Tier != OpportunityProbe {
		t.Fatalf("NO_SAMPLE should be probe: %+v", alloc["SOLUSDT"])
	}
	quality["SOLUSDT"] = historyQualityScore{Score: 0, Grade: "D"}
	alloc = AllocateLiveCapital(cfg, plan, quality, nil)
	if alloc["SOLUSDT"].Tier != OpportunityBlock || alloc["SOLUSDT"].MaxLayers != 0 {
		t.Fatalf("D quality should block: %+v", alloc["SOLUSDT"])
	}
}

func TestBuildManagedDesiredOrdersAllowsNoSampleProbeInLiveAuto(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 57, Grade: "B"}, "SOLUSDT": {Score: 0, Grade: "NO_SAMPLE"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	count := 0
	for _, d := range desired {
		if d.Symbol == "SOLUSDT" {
			count++
			if d.AllocationTier != string(OpportunityProbe) || d.Notional > cfg.Live.MaxLiveNotionalPerOrderUSDT {
				t.Fatalf("bad NO_SAMPLE probe desired: %+v", d)
			}
		}
	}
	if count != 1 {
		t.Fatalf("NO_SAMPLE SOL should get exactly one probe layer, count=%d desired=%+v blocked=%+v", count, desired, blocked)
	}
}

func TestBuildManagedDesiredOrdersUsesOpportunityAllocationNotStaticLayerNotional(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	for i := range plan.Assets {
		plan.Assets[i].ThesisID = "asset-" + plan.Assets[i].Symbol
		if len(plan.Assets[i].Layers) > 0 && plan.Assets[i].Symbol == "SOLUSDT" {
			plan.Assets[i].Layers[0].ThesisID = "layer-sol"
		}
		for j := range plan.Assets[i].Layers {
			plan.Assets[i].Layers[j].Notional = 100
		}
	}
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 20, Grade: "C"}, "SOLUSDT": {Score: 90, Grade: "A"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	if desired[0].Symbol != "SOLUSDT" {
		t.Fatalf("higher opportunity should sort first: %+v", desired)
	}
	for _, d := range desired {
		wantThesis := "asset-" + d.Symbol
		if d.Symbol == "SOLUSDT" && d.LayerIndex == 1 {
			wantThesis = "layer-sol"
		}
		if d.ThesisID != wantThesis {
			t.Fatalf("thesis provenance lost: got=%q want=%q order=%+v", d.ThesisID, wantThesis, d)
		}
		if d.Notional > cfg.Live.MaxLiveNotionalPerOrderUSDT {
			t.Fatalf("static layer notional leaked into live sizing: %+v", d)
		}
	}
}

func TestOpportunityScoreUsesCompositeInsideActiveLimitGuard(t *testing.T) {
	watch := agent2.AssetPlan{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 1, RotationScore: 1, RewardRisk: 3.5, AssetFlowScore: 1, MMScore: 100}
	watch.LiquidityQuality.Score = 100
	if got := opportunityScore(watch, historyQualityScore{Score: 100, Grade: "A"}); got != 0 {
		t.Fatalf("non ACTIVE_LIMIT score should be 0, got %.2f", got)
	}
	active := watch
	active.State = agent2.StateActiveLimit
	if got := opportunityScore(active, historyQualityScore{Score: 100, Grade: "A"}); got <= 0 {
		t.Fatalf("ACTIVE_LIMIT strong composite should score >0, got %.2f", got)
	}
	blocked := active
	blocked.SetupGates = []agent2.SetupGateResult{{Name: agent2.EntryCheckData, Pass: false, Severity: agent2.SetupGateHard, Reason: "chưa đủ dữ liệu 1D"}}
	if got := opportunityScore(blocked, historyQualityScore{Score: 100, Grade: "A"}); got != 0 {
		t.Fatalf("blocked composite score should be 0, got %.2f", got)
	}
}

func TestBTCRiskMultiplierBlocksArmedInLiveAllocator(t *testing.T) {
	if got := btcRiskMultiplier(agent1.Armed); got != 0 {
		t.Fatalf("ARMED must have zero live allocator risk budget, got %.2f", got)
	}
}

func TestBuildManagedDesiredOrdersStatePermissionMatrix(t *testing.T) {
	cfg := managedConfig()
	tests := []struct {
		name       string
		planState  agent2.State
		permission agent1.Permission
		assetState agent2.State
		want       bool
	}{
		{name: "no trade blocks", planState: agent2.StateNoTrade, permission: agent1.NoTrade, assetState: agent2.StateNoTrade},
		{name: "watch blocks", planState: agent2.StateWatch, permission: agent1.Watch, assetState: agent2.StateWatch},
		{name: "scout blocks", planState: agent2.StateScout, permission: agent1.Watch, assetState: agent2.StateScout},
		{name: "armed blocks", planState: agent2.StateArmed, permission: agent1.Armed, assetState: agent2.StateArmed},
		{name: "active with watch permission blocks", planState: agent2.StateActiveLimit, permission: agent1.Watch, assetState: agent2.StateActiveLimit},
		{name: "active with armed permission blocks", planState: agent2.StateActiveLimit, permission: agent1.Armed, assetState: agent2.StateActiveLimit},
		{name: "active plan with armed asset blocks", planState: agent2.StateActiveLimit, permission: agent1.Allowed, assetState: agent2.StateArmed},
		{name: "active limit allowed creates desired", planState: agent2.StateActiveLimit, permission: agent1.Allowed, assetState: agent2.StateActiveLimit, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := agent2.Plan{State: tt.planState, ActionPermission: tt.permission, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: tt.assetState, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 88, RewardRisk: 3.5, Reason: "matrix", Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 10}}}}}
			writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}})
			desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
			got := len(desired) > 0
			if got != tt.want {
				t.Fatalf("desired=%+v blocked=%+v wantDesired=%v", desired, blocked, tt.want)
			}
		})
	}
}

func TestAllocateLiveCapitalSubtractsPositionsAndOpenOrders(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.MaxLiveNotionalPerAssetUSDT = 6
	plan := managedPlan()
	positions := []live.LivePosition{{Symbol: "ETHUSDT", CostBasis: 4}}
	open := []live.OrderStatus{{Symbol: "ETHUSDT", Notional: 2}}
	alloc := AllocateLiveCapital(cfg, plan, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}}, positions, open)
	if alloc["ETHUSDT"].Tier != OpportunityBlock || alloc["ETHUSDT"].BudgetUSDT != 0 {
		t.Fatalf("ETH budget should be exhausted by position+open: %+v", alloc["ETHUSDT"])
	}
}

func TestBuildManagedDesiredOrderCarriesLayerAuditFields(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	plan.Assets[0].Layers[0].Invalidation = 88
	plan.Assets[0].Layers[0].Target = 130
	plan.Assets[0].Layers[0].RewardRisk = 3.5
	plan.Assets[0].Layers[0].Reason = "support retest RR 3.5"
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	found := false
	for _, d := range desired {
		if d.Symbol == "ETHUSDT" && d.LayerIndex == 1 {
			found = true
			if d.TargetPrice != 130 || d.RewardRisk != 3.5 || d.LayerReason == "" {
				t.Fatalf("missing layer audit fields: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("missing ETH layer 1: %+v", desired)
	}
}

func TestManagedCoinSummaryIncludesHardSoftBlockers(t *testing.T) {
	cfg := managedConfig()
	plan := agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateWatch, Reason: "BTC permission WATCH; không tạo probe", HardBlockers: []string{"BTC permission WATCH; không tạo probe"}, SoftBlockers: []string{"asset flow chưa reclaim/absorption"}, NextTrigger: "Chờ BTC chuyển ARMED."}}}
	got := ManageLiveOrdersDryRun(context.Background(), cfg, plan, nil, nil, nil, nil, nil, fakeHaltReader{halted: false}, true)
	eth := managedCoinForTest(t, got.PerCoin, "ETHUSDT")
	if !containsManagedString(eth.HardBlockers, "BTC permission WATCH; không tạo probe") || !containsManagedString(eth.SoftBlockers, "asset flow chưa reclaim/absorption") || len(eth.WhyNoOrder) == 0 || eth.NextTrigger == "" {
		t.Fatalf("missing blockers in summary: %+v", eth)
	}
}

func TestAssertManagedExecutionAllowedBlocksMissingDryRunProofForRealOrder(t *testing.T) {
	cfg := managedConfig()
	cfg.Live.FirstOrderRequireDryRun = true
	plan := managedPlan()
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	ctx := confirmedExecContext()
	ctx.FirstOrderDryRunApproved = false
	blockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: desired[0], DryRun: false, ManagedExecutionContext: ctx})
	if !containsManagedString(blockers, "live.first_order_require_dry_run=true; approved dry-run audit required before first real order") {
		t.Fatalf("missing dry-run proof blocker: %v", blockers)
	}
}

func TestAssertManagedExecutionAllowedBlocksWrongBTCAccumulation(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) == 0 {
		t.Fatalf("bad desired=%+v blocked=%+v", desired, blocked)
	}
	ctx := confirmedExecContext()
	ctx.BTCAccumulationPhase = "MARKDOWN"
	blockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: desired[0], DryRun: false, ManagedExecutionContext: ctx})
	if !containsManagedString(blockers, "BTC accumulation phase must be ACCUMULATION_CONFIRMED") {
		t.Fatalf("missing BTC phase blocker: %v", blockers)
	}
}

func TestManageLiveOrdersFinalAssertionBlocksMissingBTCPhaseBeforeSubmit(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	ex := &fakeManagedExchange{}
	rec := &fakeManagedRecorder{}
	got := ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false}, ManagedExecutionContext{}, rec, false)
	if len(ex.placed) != 0 || len(rec.submitted) != 0 || len(got.Blocked) == 0 {
		t.Fatalf("missing BTC phase should block before exchange submit: placed=%d submitted=%d result=%+v", len(ex.placed), len(rec.submitted), got)
	}
}

var historyQualityReportTestMu sync.Mutex

func writeHistoryQualityReportForTest(t *testing.T, scores map[string]historyQualityScore) {
	t.Helper()
	historyQualityReportTestMu.Lock()
	t.Cleanup(historyQualityReportTestMu.Unlock)
	perCoin := map[string]map[string]any{}
	for symbol, score := range scores {
		perCoin[symbol] = map[string]any{"quality_score": score.Score, "quality_grade": score.Grade}
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"per_coin": perCoin})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("reports/live_manager_history_latest.json", b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove("reports/live_manager_history_latest.json") })
}

func managedCoinForTest(t *testing.T, coins []ManagedCoinSummary, symbol string) ManagedCoinSummary {
	t.Helper()
	for _, coin := range coins {
		if coin.Symbol == symbol {
			return coin
		}
	}
	t.Fatalf("missing coin summary %s in %+v", symbol, coins)
	return ManagedCoinSummary{}
}

func containsManagedString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestShouldCancelOpenOrderStale(t *testing.T) {
	cfg := config.Config{}
	cfg.Live.CancelStaleAfterMinutes = 240
	plan := agent2.Plan{State: agent2.StateActiveLimit}
	desired := ManagedDesiredOrder{Symbol: "ETHUSDT", Price: 1800}
	order := live.OrderStatus{Symbol: "ETHUSDT", SubmittedAt: time.Now().Add(-5 * time.Hour).Unix()}
	if !shouldCancelOpenOrder(cfg, plan, order, desired) {
		t.Fatal("expected stale order to be cancelled")
	}
}

func TestShouldNotCancelOpenOrderWithinTimeout(t *testing.T) {
	cfg := config.Config{}
	cfg.Live.CancelStaleAfterMinutes = 240
	plan := agent2.Plan{State: agent2.StateActiveLimit}
	desired := ManagedDesiredOrder{Symbol: "ETHUSDT", Price: 1800}
	order := live.OrderStatus{Symbol: "ETHUSDT", SubmittedAt: time.Now().Add(-1 * time.Hour).Unix()}
	if shouldCancelOpenOrder(cfg, plan, order, desired) {
		t.Fatal("expected order within timeout to be kept")
	}
}

func TestBuildManagedDesiredOrdersBlocksUnboundDCAPlannerBuy(t *testing.T) {
	cfg := managedConfig()
	cfg.DCA.AllocationEnabled = true
	plan := managedPlan()
	plan.Assets = plan.Assets[:1]
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(desired) != 0 || len(blocked) != 2 {
		t.Fatalf("desired=%+v blocked=%+v", desired, blocked)
	}
	for _, b := range blocked {
		if b.Reason != "DCA thesis binding required" {
			t.Fatalf("blocked=%+v", blocked)
		}
	}
}
