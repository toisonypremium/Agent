package main

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

func readinessTestConfig() config.Config {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.35, "SOLUSDT": 0.45, "RENDERUSDT": 0.20}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.70
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	cfg.Execution.LayerDistribution = []float64{0.25, 0.35, 0.40}
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	return cfg
}

func TestAccumulationReadinessMarkdownShowsThreeTargetsAndNoBTCAsset(t *testing.T) {
	cfg := readinessTestConfig()
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Watch, MarketRegime: "RANGE", RiskLevel: agent1.Medium, TrendScore: 45}
	plan := agent2.Plan{State: agent2.StateWatch, Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{
		readinessCandidate("SOLUSDT", 0.49, 1),
		readinessCandidate("ETHUSDT", 0.48, 2),
		readinessCandidate("RENDERUSDT", 0.47, 3),
	}}}
	report := buildAccumulationReadinessReport(cfg, analysis, plan)
	md := accumulationReadinessMarkdown(report)
	for _, want := range []string{"ACCUMULATION READINESS", "BTC gate:", "BTC là market gate/benchmark, không phải target gom", "SOLUSDT", "ETHUSDT", "RENDERUSDT", "Hard fails: BTC_PERMISSION", "Soft waits: ASSET_FLOW_ENTRY", "Preview layers if BTC gate opens (not orders):"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "BTCUSDT —") || strings.Contains(md, "BTCUSDT 0%") {
		t.Fatalf("BTC must not appear as accumulation target:\n%s", md)
	}
}

func TestReadinessLayerPreviewUsesBudgetCapAndFractions(t *testing.T) {
	cfg := readinessTestConfig()
	cfg.Portfolio.Allocation["SOLUSDT"] = 0.90
	candidate := readinessCandidate("SOLUSDT", 0.50, 1)
	layers := readinessLayerPreview(cfg, candidate)
	if len(layers) != 3 {
		t.Fatalf("expected 3 preview layers: %+v", layers)
	}
	// Budget would be 630, capped at 450 by max_single_asset_deployment.
	want := []float64{112.5, 157.5, 180.0}
	for i := range layers {
		if layers[i].Notional != want[i] {
			t.Fatalf("layer %d notional=%v want %v", i+1, layers[i].Notional, want[i])
		}
		if layers[i].RewardRisk <= 0 || layers[i].Quantity <= 0 {
			t.Fatalf("invalid layer preview: %+v", layers[i])
		}
	}
}

func readinessCandidate(symbol string, readiness float64, rank int) agent2.WatchCandidate {
	return agent2.WatchCandidate{
		Symbol:           symbol,
		State:            agent2.StateWatch,
		ReadinessScore:   readiness,
		Tier:             agent2.WatchTierEarly,
		RotationRank:     rank,
		RotationScore:    0.80 - float64(rank)*0.10,
		RelativeReturn:   0.05,
		FlowBias:         flow.BiasNeutral,
		FlowBullScore:    0,
		MMCase:           agent2.MMCaseNoEdge,
		MMScore:          20,
		Price:            82,
		Support:          market.Zone{Name: "support", Low: 60, High: 63},
		Resistance:       market.Zone{Name: "resistance", Low: 95, High: 98},
		DiscountGap:      0.30,
		ZoneWidthPct:     0.05,
		ZoneQuality:      "ZONE_OK",
		RewardRisk:       0.7,
		LiquidityQuality: liquidity.Quality{Pass: true, Grade: "A"},
		Missing:          []string{"BTC permission WATCH; không tạo probe", "asset flow chưa reclaim/absorption"},
		NextTrigger:      "Chờ BTC chuyển ALLOWED; asset chỉ nằm watchlist, không tạo lệnh.",
		EntryChecklist: []agent2.EntryChecklistItem{
			{Name: agent2.EntryCheckBTCPermission, Pass: false, Severity: agent2.EntryCheckHard, Reason: "BTC permission WATCH"},
			{Name: agent2.EntryCheckAssetFlowEntry, Pass: false, Severity: agent2.EntryCheckSoft, Reason: "asset flow chưa reclaim/absorption"},
		},
	}
}
