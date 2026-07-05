package liveguard

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

type fakeManagedExchange struct {
	placed   []live.LimitOrderRequest
	canceled []live.CancelOrderRequest
}

func (f *fakeManagedExchange) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.placed = append(f.placed, req)
	return live.OrderResult{InstID: req.InstID, OrderID: "ord", ClientOrderID: req.ClientOrderID, Submitted: true}, nil
}

func (f *fakeManagedExchange) CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	f.canceled = append(f.canceled, req)
	return live.CancelOrderResult{InstID: req.InstID, OrderID: req.OrderID, ClientOrderID: req.ClientOrderID, Canceled: true, Code: "0"}, nil
}

func TestManageLiveOrdersAllowsMultipleAssetsAndLayers(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	ex := &fakeManagedExchange{}
	got := ManageLiveOrders(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false})
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
	got := ManageLiveOrders(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Canceled) != 1 || len(ex.canceled) != 1 {
		t.Fatalf("expected cancel, got %+v", got)
	}
}

func TestManageLiveOrdersKeepsMatchingOrder(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	ex := &fakeManagedExchange{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Symbol: "ETHUSDT", ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.02, Notional: 2, LayerIndex: 1}}
	got := ManageLiveOrders(context.Background(), cfg, plan, open, nil, nil, ex, ex, fakeHaltReader{halted: false})
	if len(got.Kept) != 1 || len(got.Canceled) != 0 || len(ex.canceled) != 0 {
		t.Fatalf("expected keep, got %+v", got)
	}
}

func managedConfig() config.Config {
	var cfg config.Config
	cfg.Live.Enabled = true
	cfg.Live.AutoExecute = true
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.OrderManagementEnabled = true
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2
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
	got := ManageLiveOrders(context.Background(), cfg, plan, nil, nil, nil, ex, ex, fakeHaltReader{halted: false})
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

func TestBuildManagedDesiredOrdersSkipsDGradeInCanary(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 57, Grade: "B"}, "SOLUSDT": {Score: 0, Grade: "D"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	for _, d := range desired {
		if d.Symbol == "SOLUSDT" {
			t.Fatalf("D-grade SOL should be skipped in canary: %+v", desired)
		}
	}
	foundBlock := false
	for _, b := range blocked {
		if b.Symbol == "SOLUSDT" && b.Reason == "canary quality filter blocked D-grade coin" {
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

func TestBuildManagedDesiredOrdersAllowsNoSampleProbeInCanary(t *testing.T) {
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
		if d.Notional > cfg.Live.MaxLiveNotionalPerOrderUSDT {
			t.Fatalf("static layer notional leaked into live sizing: %+v", d)
		}
	}
}

func TestBuildManagedDesiredOrdersAllowsArmedProbeAsset(t *testing.T) {
	cfg := managedConfig()
	plan := agent2.Plan{State: agent2.StateArmed, ActionPermission: agent1.Armed, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateArmed, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 88, RewardRisk: 3.5, Reason: "armed probe", Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 10}}}}}
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 0, Grade: "NO_SAMPLE"}})
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, nil, nil, nil)
	if len(blocked) != 0 || len(desired) != 1 {
		t.Fatalf("expected one ARMED probe desired, desired=%+v blocked=%+v", desired, blocked)
	}
	if desired[0].AllocationTier != string(OpportunityProbe) || desired[0].Notional > cfg.Live.MaxLiveNotionalPerOrderUSDT {
		t.Fatalf("bad ARMED probe sizing: %+v", desired[0])
	}
}

func writeHistoryQualityReportForTest(t *testing.T, scores map[string]historyQualityScore) {
	t.Helper()
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
