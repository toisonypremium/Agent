package agent2

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/market"
)

func TestBuildWatchlistRanksClosestCandidate(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.MinRewardRisk = 0.1
	btc := scoreCandles("BTCUSDT", 100, 101, 80)
	assets := map[string][]market.Candle{
		"ETHUSDT":    scoreCandles("ETHUSDT", 100, 100, 80),
		"SOLUSDT":    scoreCandles("SOLUSDT", 100, 100, 80),
		"RENDERUSDT": scoreCandles("RENDERUSDT", 100, 100, 80),
	}
	plans := []AssetPlan{
		{Symbol: "ETHUSDT", State: StateWatch, RotationRank: 1, RotationScore: 0.80, Reason: "asset flow entry chưa xác nhận: bias=NEUTRAL bull=0.00 bear=0.00"},
		{Symbol: "SOLUSDT", State: StateWatch, RotationRank: 2, RotationScore: 0.60, Reason: "giá chưa vào discount zone"},
		{Symbol: "RENDERUSDT", State: StateNoTrade, RotationRank: 3, RotationScore: 0.30, Reason: "relative strength filter chặn asset"},
	}
	rotation := []AssetRotationScore{{Symbol: "ETHUSDT", Rank: 1, Score: 0.80, Eligible: true}, {Symbol: "SOLUSDT", Rank: 2, Score: 0.60, Eligible: true}, {Symbol: "RENDERUSDT", Rank: 3, Score: 0.30, Eligible: false}}
	got := BuildWatchlist(cfg, assets, btc, rotation, plans)
	if len(got.Candidates) != 3 {
		t.Fatalf("expected candidates: %+v", got)
	}
	if got.Candidates[0].ReadinessScore < got.Candidates[1].ReadinessScore {
		t.Fatalf("watchlist not sorted by readiness: %+v", got.Candidates)
	}
}

func TestBuildWatchlistExplainsMissingFlow(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}
	btc := assetCandles(80, false)
	plans := []AssetPlan{{Symbol: "ETHUSDT", State: StateWatch, Reason: "asset flow entry chưa xác nhận: bias=NEUTRAL bull=0.00 bear=0.00"}}
	got := BuildWatchlist(cfg, assets, btc, nil, plans)
	if len(got.Candidates) == 0 {
		t.Fatalf("expected candidate: %+v", got)
	}
	c := got.Candidates[0]
	if !containsString(c.Missing, "asset flow chưa reclaim/absorption") {
		t.Fatalf("expected flow missing: %+v", c)
	}
	if nextTrigger(WatchCandidate{Missing: []string{"asset flow chưa reclaim/absorption"}}) == "" {
		t.Fatalf("expected flow next trigger")
	}
}

func TestBuildPlanWithBenchmarksIncludesWatchlistWhenAgent1NotAllowed(t *testing.T) {
	cfg := testConfig()
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true), "SOLUSDT": assetCandles(80, true), "RENDERUSDT": assetCandles(80, true)}
	btc := assetCandles(80, false)
	got := BuildPlanWithBenchmarks(cfg, analysis, assets, map[string][]market.Candle{"BTCUSDT": btc})
	if got.State != StateWatch || len(got.Assets) != 0 || len(got.Watchlist.Candidates) == 0 {
		t.Fatalf("expected watchlist on non-ALLOWED BTC: %+v", got)
	}
	if !containsString(got.Watchlist.Candidates[0].Missing, "BTC permission chưa ALLOWED") {
		t.Fatalf("expected BTC permission missing: %+v", got.Watchlist.Candidates[0])
	}
}

func TestWatchlistCapsBTCNotAllowedNoise(t *testing.T) {
	cfg := testConfig()
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true), "SOLUSDT": assetCandles(80, true), "RENDERUSDT": assetCandles(80, true)}
	btc := assetCandles(80, false)
	got := BuildPlanWithBenchmarks(cfg, analysis, assets, map[string][]market.Candle{"BTCUSDT": btc})
	for _, c := range got.Watchlist.Candidates {
		if c.Actionable || c.Tier != WatchTierEarly || c.ReadinessScore > 0.49 {
			t.Fatalf("BTC-not-allowed candidate should be capped: %+v", c)
		}
	}
}

func TestWatchlistCapsRelativeWeakAsBlocked(t *testing.T) {
	cfg := testConfig()
	c := tuneWatchCandidate(WatchCandidate{Symbol: "ETHUSDT", ReadinessScore: 0.90, Missing: []string{"relative strength yếu hơn BTC"}}, cfg)
	if c.Actionable || c.Tier != WatchTierBlocked || c.ReadinessScore > 0.35 {
		t.Fatalf("relative weak should be blocked/capped: %+v", c)
	}
}

func TestSummaryIncludesWatchlist(t *testing.T) {
	p := Plan{State: StateWatch, Watchlist: WatchlistReport{Candidates: []WatchCandidate{{Symbol: "ETHUSDT", ReadinessScore: 0.62, Tier: WatchTierEarly, Missing: []string{"asset flow chưa reclaim/absorption"}, EntryChecklist: []EntryChecklistItem{{Name: EntryCheckAssetFlowEntry, Pass: false, Severity: EntryCheckSoft, Reason: "asset flow chưa reclaim/absorption"}}, NextTrigger: "Chờ sweep low + reclaim support."}}}}
	got := Summary(p)
	if !strings.Contains(got, "Watchlist") && !strings.Contains(got, "gần đạt điều kiện") {
		t.Fatalf("summary missing watchlist: %s", got)
	}
	if !strings.Contains(got, "checklist") {
		t.Fatalf("summary missing checklist: %s", got)
	}
}

func TestBuildWatchlistIncludesEntryChecklist(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}
	btc := assetCandles(80, false)
	plans := []AssetPlan{{Symbol: "ETHUSDT", State: StateWatch, Reason: "asset flow entry chưa xác nhận: bias=NEUTRAL bull=0.00 bear=0.00"}}
	got := BuildWatchlist(cfg, assets, btc, nil, plans)
	if len(got.Candidates) == 0 || len(got.Candidates[0].EntryChecklist) == 0 {
		t.Fatalf("expected checklist: %+v", got)
	}
	for _, name := range []string{EntryCheckBTCPermission, EntryCheckAssetFlowEntry, EntryCheckDiscountZone, EntryCheckRewardRisk} {
		if _, ok := checklistItem(got.Candidates[0].EntryChecklist, name); !ok {
			t.Fatalf("missing checklist item %s: %+v", name, got.Candidates[0].EntryChecklist)
		}
	}
}

func TestEntryChecklistMarksBTCMissingHardFail(t *testing.T) {
	cfg := testConfig()
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}
	btc := assetCandles(80, false)
	got := BuildPlanWithBenchmarks(cfg, analysis, assets, map[string][]market.Candle{"BTCUSDT": btc})
	item, ok := checklistItem(got.Watchlist.Candidates[0].EntryChecklist, EntryCheckBTCPermission)
	if !ok || item.Pass || item.Severity != EntryCheckHard {
		t.Fatalf("BTC checklist should hard fail: %+v", got.Watchlist.Candidates[0].EntryChecklist)
	}
}

func TestEntryChecklistMarksFlowUnconfirmedSoftFail(t *testing.T) {
	c := WatchCandidate{Missing: []string{"asset flow chưa reclaim/absorption"}}
	items := buildEntryChecklist(c, testConfig())
	item, ok := checklistItem(items, EntryCheckAssetFlowEntry)
	if !ok || item.Pass || item.Severity != EntryCheckSoft {
		t.Fatalf("flow checklist should soft fail: %+v", items)
	}
}

func TestChecklistSummaryGroupsFailures(t *testing.T) {
	got := ChecklistSummary([]EntryChecklistItem{{Name: EntryCheckBTCPermission, Pass: false, Severity: EntryCheckHard}, {Name: EntryCheckAssetFlowEntry, Pass: false, Severity: EntryCheckSoft}})
	if !strings.Contains(got, "HARD fail") || !strings.Contains(got, "SOFT wait") {
		t.Fatalf("unexpected checklist summary: %s", got)
	}
}

func checklistItem(items []EntryChecklistItem, name string) (EntryChecklistItem, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return EntryChecklistItem{}, false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
