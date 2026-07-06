package liveguard

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"

	"math"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

func TestRunLiveManagerHistorySimulationRejectsShortData(t *testing.T) {
	cfg := historyTestConfig()
	_, err := RunLiveManagerHistorySimulation(cfg, map[string][]market.Candle{"1d": historyCandles("BTCUSDT", 10, 100, nil)}, map[string][]market.Candle{"ETHUSDT": historyCandles("ETHUSDT", 10, 10, nil)})
	if err == nil || !strings.Contains(err.Error(), "not enough BTC 1d candles") {
		t.Fatalf("expected short data error, got %v", err)
	}
}

func TestHistoryOrdersDoNotFillOnPlacementCandle(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{}}
	open := []historyOpenOrder{}
	desired := ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2}
	cycle := ManagedCycleResult{Placed: []ManagedOrderDecision{{Action: "would_place", Symbol: "ETHUSDT", LayerIndex: 1, Desired: desired, Reason: "missing active accumulation layer order"}}}
	applyHistoryManagerResult(&result, &open, cycle, nil, 5, "2026-01-05")
	assets := map[string][]market.Candle{"ETHUSDT": historyCandles("ETHUSDT", 8, 110, map[int]float64{5: 90, 6: 90})}
	processHistoryFills(&result, &open, assets, 5, "2026-01-05")
	if result.PerCoin["ETHUSDT"].Filled != 0 || len(open) != 1 {
		t.Fatalf("same-candle fill occurred: stats=%+v open=%d", result.PerCoin["ETHUSDT"], len(open))
	}
	processHistoryFills(&result, &open, assets, 6, "2026-01-06")
	if result.PerCoin["ETHUSDT"].Filled != 1 || len(open) != 0 {
		t.Fatalf("next-candle fill missing: stats=%+v open=%d", result.PerCoin["ETHUSDT"], len(open))
	}
}

func TestFinalizeHistoryStatsFiniteRatesAndBestLayer(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{"ETHUSDT": {Placed: 4, Canceled: 1, Replaced: 1, Filled: 2, LayerFills: map[int]int{2: 2, 1: 1}}, "SOLUSDT": {LayerFills: map[int]int{}}}}
	result.Total = LiveManagerHistoryStats{Placed: 4, Canceled: 1, Replaced: 1, Filled: 2, LayerFills: map[int]int{2: 2, 1: 1}}
	finalizeHistoryStats(&result)
	eth := result.PerCoin["ETHUSDT"]
	if eth.FillRate != 0.5 || eth.CancelRate != 0.25 || eth.ReplaceRate != 0.25 || eth.BestLayer != 2 || eth.QualityGrade == "" {
		t.Fatalf("bad finalized ETH stats: %+v", eth)
	}
	sol := result.PerCoin["SOLUSDT"]
	if math.IsNaN(sol.FillRate) || math.IsInf(sol.FillRate, 0) || sol.BestLayer != 0 {
		t.Fatalf("bad zero-placement SOL stats: %+v", sol)
	}
}

func TestHistoryStatsIncludeConfiguredAssets(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{}}
	ensureHistoryStats(&result, "ETHUSDT")
	ensureHistoryStats(&result, "SOLUSDT")
	if _, ok := result.PerCoin["ETHUSDT"]; !ok {
		t.Fatalf("missing ETH stats")
	}
	if _, ok := result.PerCoin["SOLUSDT"]; !ok {
		t.Fatalf("missing SOL stats")
	}
}

func TestHistoryQualityScoresGoodAndBadSamples(t *testing.T) {
	good := LiveManagerHistoryStats{Placed: 10, Filled: 7, Canceled: 0, Replaced: 1, Expired: 1}
	finalizeOneHistoryStats(&good)
	if good.QualityGrade != "B" || good.QualityScore <= 0 {
		t.Fatalf("bad good quality: %+v", good)
	}
	bad := LiveManagerHistoryStats{Placed: 3, Filled: 0, Canceled: 3}
	finalizeOneHistoryStats(&bad)
	if bad.QualityGrade != "D" || bad.QualityScore != 0 {
		t.Fatalf("bad poor quality: %+v", bad)
	}
}

func TestHistoryCancelReasonsCountPerCoinAndTotal(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{}}
	cycle := ManagedCycleResult{Canceled: []ManagedOrderDecision{{Symbol: "ETHUSDT", Reason: "plan no longer ACTIVE_LIMIT"}, {Symbol: "ETHUSDT", Reason: "plan no longer ACTIVE_LIMIT"}, {Symbol: "SOLUSDT", Reason: "order no longer matches current desired layer"}}}
	recordHistoryCycle(&result, cycle, agent2.Plan{State: agent2.StateWatch})
	if result.PerCoin["ETHUSDT"].CancelReasons["PLAN_NO_LONGER_ACTIVE_LIMIT"] != 2 {
		t.Fatalf("bad ETH cancel reasons: %+v", result.PerCoin["ETHUSDT"].CancelReasons)
	}
	if result.PerCoin["SOLUSDT"].CancelReasons["ORDER_NO_LONGER_MATCHES_CURRENT_DESIRED_LAYER"] != 1 {
		t.Fatalf("bad SOL cancel reasons: %+v", result.PerCoin["SOLUSDT"].CancelReasons)
	}
	if result.Total.CancelReasons["PLAN_NO_LONGER_ACTIVE_LIMIT"] != 2 || result.Total.CancelReasons["ORDER_NO_LONGER_MATCHES_CURRENT_DESIRED_LAYER"] != 1 {
		t.Fatalf("bad total cancel reasons: %+v", result.Total.CancelReasons)
	}
}

func TestHistoryDesiredLossExplainsLostAsset(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{}}
	cycle := ManagedCycleResult{Canceled: []ManagedOrderDecision{{Symbol: "SOLUSDT", Reason: "order no longer matches active asset/layer"}}}
	plan := agent2.Plan{State: agent2.StateWatch, Assets: []agent2.AssetPlan{{Symbol: "SOLUSDT", State: agent2.StateWatch, Reason: "reward/risk 1.20 thấp hơn 3.00"}}}
	recordHistoryCycle(&result, cycle, plan)
	if result.PerCoin["SOLUSDT"].DesiredLoss["REWARD/RISK_1.20_THẤP_HƠN_3.00"] != 1 {
		t.Fatalf("bad SOL desired loss: %+v", result.PerCoin["SOLUSDT"].DesiredLoss)
	}
	if result.Total.DesiredLoss["REWARD/RISK_1.20_THẤP_HƠN_3.00"] != 1 {
		t.Fatalf("bad total desired loss: %+v", result.Total.DesiredLoss)
	}
}

func TestProductionArmedProbeModeAddsNote(t *testing.T) {
	result := LiveManagerHistoryResult{ProductionArmedProbe: true, PerCoin: map[string]LiveManagerHistoryStats{}, Notes: []string{}}
	cycle := ManagedCycleResult{Desired: []ManagedDesiredOrder{{Symbol: "ETHUSDT", AllocationTier: string(OpportunityProbe)}}, Placed: []ManagedOrderDecision{{Symbol: "ETHUSDT", Desired: ManagedDesiredOrder{Symbol: "ETHUSDT", AllocationTier: string(OpportunityProbe)}}}}
	recordArmedProbeCycle(&result, cycle, agent2.Plan{State: agent2.StateArmed})
	finalizeHistoryStats(&result)
	if result.ArmedProbe.Desired != 1 || result.ArmedProbe.Placed != 1 {
		t.Fatalf("bad armed probe stats: %+v", result.ArmedProbe)
	}
}

func TestProductionArmedProbeIgnoresWatchCycle(t *testing.T) {
	result := LiveManagerHistoryResult{ProductionArmedProbe: true, PerCoin: map[string]LiveManagerHistoryStats{}}
	cycle := ManagedCycleResult{Desired: []ManagedDesiredOrder{{Symbol: "ETHUSDT"}}, Placed: []ManagedOrderDecision{{Symbol: "ETHUSDT"}}}
	recordArmedProbeCycle(&result, cycle, agent2.Plan{State: agent2.StateWatch})
	if result.ArmedProbe.Desired != 0 || result.ArmedProbe.Placed != 0 {
		t.Fatalf("WATCH cycle must not count as armed probe: %+v", result.ArmedProbe)
	}
}

func historyTestConfig() config.Config {
	var cfg config.Config
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Execution.OrderExpiryHours = 48
	return cfg
}

func historyCandles(symbol string, n int, price float64, lows map[int]float64) []market.Candle {
	out := make([]market.Candle, 0, n)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		low := price * 0.99
		if lows != nil && lows[i] > 0 {
			low = lows[i]
		}
		out = append(out, market.Candle{Symbol: symbol, Interval: "1d", OpenTime: start.AddDate(0, 0, i), CloseTime: start.AddDate(0, 0, i+1), Open: price, High: price * 1.02, Low: low, Close: price, Volume: 1000})
	}
	return out
}

func TestHistoryResearchModeTreatsArmedAsAllowed(t *testing.T) {
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Armed, Timestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}
	result := LiveManagerHistoryResult{}
	got := applyHistoryResearchMode(&result, analysis, LiveManagerHistoryOptions{ResearchArmed: true}, "2026-01-02")
	if got.ActionPermission != agent1.Allowed {
		t.Fatalf("expected ALLOWED, got %s", got.ActionPermission)
	}
	if len(result.Events) != 1 || result.Events[0].Type != "RESEARCH_ARMED_AS_ALLOWED" {
		t.Fatalf("expected research event, got %+v", result.Events)
	}
}

func TestHistoryResearchModeLeavesWatchUnchanged(t *testing.T) {
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Watch}
	got := applyHistoryResearchMode(&LiveManagerHistoryResult{}, analysis, LiveManagerHistoryOptions{ResearchArmed: true}, "2026-01-02")
	if got.ActionPermission != agent1.Watch {
		t.Fatalf("expected WATCH unchanged, got %s", got.ActionPermission)
	}
}

func TestHistoryPlanBlockersCountChecklistFailures(t *testing.T) {
	result := LiveManagerHistoryResult{PerCoin: map[string]LiveManagerHistoryStats{}}
	plan := agent2.Plan{Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{{
		Symbol: "ETHUSDT",
		State:  agent2.StateWatch,
		EntryChecklist: []agent2.EntryChecklistItem{
			{Name: agent2.EntryCheckDiscountZone, Severity: agent2.EntryCheckSoft, Pass: false, Reason: "giá chưa vào discount zone"},
			{Name: agent2.EntryCheckRewardRisk, Severity: agent2.EntryCheckSoft, Pass: false, Reason: "reward/risk dưới 3"},
		},
	}}}}
	recordHistoryPlanBlockers(&result, plan)
	stats := result.PerCoin["ETHUSDT"]
	if stats.Blockers["SOFT:DISCOUNT_ZONE"] != 1 || stats.Blockers["SOFT:REWARD_RISK"] != 1 {
		t.Fatalf("bad blockers: %+v", stats.Blockers)
	}
	if result.Total.Blockers["SOFT:DISCOUNT_ZONE"] != 1 {
		t.Fatalf("bad total blockers: %+v", result.Total.Blockers)
	}
}

func TestNormalizeHistoryBlocker(t *testing.T) {
	cases := map[string]string{
		"BTC permission chưa ALLOWED":        "BTC_PERMISSION",
		"giá chưa vào discount zone":         "DISCOUNT_ZONE",
		"reward/risk dưới 3":                 "REWARD_RISK",
		"asset flow chưa reclaim/absorption": "ASSET_FLOW_ENTRY",
	}
	for in, want := range cases {
		if got := normalizeHistoryBlocker(in); got != want {
			t.Fatalf("normalize %q=%q want %q", in, got, want)
		}
	}
}

func TestHistoryResearchProfileFlowSoftDisablesAssetFlowOnly(t *testing.T) {
	cfg := historyTestConfig()
	cfg.Risk.DisableAssetFlowEntryFilter = false
	cfg.Risk.MinAssetFlowBullScore = 0.25
	got := applyHistoryResearchProfile(cfg, LiveManagerHistoryOptions{ResearchProfile: "flow-soft"})
	if !got.Risk.DisableAssetFlowEntryFilter {
		t.Fatalf("flow-soft should disable asset flow entry filter")
	}
	if got.Risk.MinAssetFlowBullScore > 0.10 {
		t.Fatalf("flow-soft should lower min bull score, got %.2f", got.Risk.MinAssetFlowBullScore)
	}
	if got.Risk.DisableRelativeStrengthFilter {
		t.Fatalf("flow-soft should not disable relative strength")
	}
}

func TestHistoryResearchProfileDiscountSoftRelaxesDiscountOnly(t *testing.T) {
	cfg := historyTestConfig()
	cfg.Risk.DiscountZonePremiumPct = 0.05
	got := applyHistoryResearchProfile(cfg, LiveManagerHistoryOptions{ResearchProfile: "discount-soft"})
	if got.Risk.DiscountZonePremiumPct != 0.10 {
		t.Fatalf("discount-soft premium=%.2f", got.Risk.DiscountZonePremiumPct)
	}
	if got.Risk.DisableAssetFlowEntryFilter {
		t.Fatalf("discount-soft should not disable asset flow")
	}
	if got.Risk.DisableRelativeStrengthFilter {
		t.Fatalf("discount-soft should not disable relative strength")
	}
}

func TestHistoryResearchProfileEntrySoftRelaxesEntryGates(t *testing.T) {
	cfg := historyTestConfig()
	cfg.Risk.MinAssetFlowBullScore = 0.25
	cfg.Risk.DiscountZonePremiumPct = 0.05
	cfg.Risk.DisableRotationScoreFilter = false
	cfg.Risk.MaxRotationRank = 3
	cfg.Risk.DisableRelativeStrengthFilter = false
	got := applyHistoryResearchProfile(cfg, LiveManagerHistoryOptions{ResearchProfile: "entry-soft"})
	if !got.Risk.DisableAssetFlowEntryFilter {
		t.Fatalf("entry-soft should disable asset flow entry")
	}
	if got.Risk.DiscountZonePremiumPct != 0.10 {
		t.Fatalf("entry-soft discount premium=%.2f", got.Risk.DiscountZonePremiumPct)
	}
	if !got.Risk.DisableRotationScoreFilter || got.Risk.MaxRotationRank != 0 {
		t.Fatalf("entry-soft should relax rotation, got disable=%v rank=%d", got.Risk.DisableRotationScoreFilter, got.Risk.MaxRotationRank)
	}
	if !got.Risk.DisableRelativeStrengthFilter {
		t.Fatalf("entry-soft should disable relative strength")
	}
}

func TestHistoryResearchExpiryDaysOverridesConfigExpiry(t *testing.T) {
	cfg := historyTestConfig()
	cfg.Live.CancelStaleAfterMinutes = 180
	cfg.Execution.OrderExpiryHours = 48
	if got := historyExpiryDays(cfg, LiveManagerHistoryOptions{}); got != 1 {
		t.Fatalf("config expiry days=%d", got)
	}
	if got := historyExpiryDays(cfg, LiveManagerHistoryOptions{ResearchExpiryDays: 7}); got != 7 {
		t.Fatalf("research expiry days=%d", got)
	}
}

func TestHistoryHoldThroughWatchKeepsPlanInactiveCancel(t *testing.T) {
	cycle := ManagedCycleResult{Canceled: []ManagedOrderDecision{{Action: "would_cancel", Symbol: "ETHUSDT", LayerIndex: 1, Order: liveOrderStatusForHistoryTest("ETHUSDT", 1), Reason: "plan no longer ACTIVE_LIMIT"}}}
	got := applyHistoryHoldThroughWatch(cycle, agent2.Plan{State: agent2.StateWatch}, LiveManagerHistoryOptions{ResearchHoldWatch: true}, nil)
	if len(got.Canceled) != 0 || len(got.Kept) != 1 {
		t.Fatalf("expected cancel converted to keep, got canceled=%d kept=%d", len(got.Canceled), len(got.Kept))
	}
	if got.Kept[0].Reason != "research hold-through-watch: plan WATCH, keep existing simulated order" {
		t.Fatalf("bad keep reason: %s", got.Kept[0].Reason)
	}
}

func TestHistoryHoldThroughWatchDoesNotKeepActiveLimitReplace(t *testing.T) {
	cycle := ManagedCycleResult{Replaced: []ManagedOrderDecision{{Action: "would_cancel", Symbol: "ETHUSDT", LayerIndex: 1, Order: liveOrderStatusForHistoryTest("ETHUSDT", 1), Desired: ManagedDesiredOrder{Symbol: "ETHUSDT", LayerIndex: 1, Price: 90}, Reason: "order no longer matches current desired layer", ReplacedOrder: true}}}
	got := applyHistoryHoldThroughWatch(cycle, agent2.Plan{State: agent2.StateWatch}, LiveManagerHistoryOptions{ResearchHoldWatch: true}, nil)
	if len(got.Replaced) != 1 || len(got.Kept) != 0 {
		t.Fatalf("expected replace preserved, got replaced=%d kept=%d", len(got.Replaced), len(got.Kept))
	}
}

func TestHistoryHoldAboveDiscountKeepsNearDiscountLoss(t *testing.T) {
	cycle := ManagedCycleResult{Canceled: []ManagedOrderDecision{{Action: "would_cancel", Symbol: "SOLUSDT", LayerIndex: 1, Order: liveOrderStatusForHistoryTest("SOLUSDT", 1), Reason: "order no longer matches active asset/layer"}}}
	assets := map[string][]market.Candle{"SOLUSDT": historyCandles("SOLUSDT", 60, 100, nil)}
	got := applyHistoryHoldThroughWatch(cycle, agent2.Plan{State: agent2.StateWatch}, LiveManagerHistoryOptions{ResearchHoldWatch: true, ResearchHoldPriceAboveDiscountPct: 0.03}, assets)
	if len(got.Canceled) != 0 || len(got.Kept) != 1 {
		t.Fatalf("expected near-discount cancel converted to keep, got canceled=%d kept=%d", len(got.Canceled), len(got.Kept))
	}
	if got.Kept[0].Reason != "research hold-if-price-above-discount: price still near discount zone" {
		t.Fatalf("bad keep reason: %s", got.Kept[0].Reason)
	}
}

func TestHistoryCancelDiagnosticsIncludesDistance(t *testing.T) {
	decision := ManagedOrderDecision{Symbol: "SOLUSDT", Order: liveOrderStatusForHistoryTest("SOLUSDT", 1)}
	assets := map[string][]market.Candle{"SOLUSDT": historyCandles("SOLUSDT", 60, 100, nil)}
	got := historyCancelDiagnostics(decision, assets)
	if !strings.Contains(got, "close=") || !strings.Contains(got, "support_high=") || !strings.Contains(got, "distance_above_support=") || !strings.Contains(got, "order_price=") {
		t.Fatalf("missing diagnostics: %s", got)
	}
}

func liveOrderStatusForHistoryTest(symbol string, layer int) live.OrderStatus {
	return live.OrderStatus{Symbol: symbol, InstID: "ETH-USDT", LayerIndex: layer, Price: 100, Notional: 2}
}
