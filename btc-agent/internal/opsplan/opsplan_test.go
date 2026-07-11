package opsplan

import (
	"strings"
	"testing"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestBuildCapitalOnlyExecutesActiveAllowedAndKeepsReserve(t *testing.T) {
	var cfg config.Config
	cfg.App.Mode = "paper"
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.ReserveCashRatio = 0.10
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.40, "SOLUSDT": 0.40, "RENDERUSDT": 0.20}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.50
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	cfg.Execution.LayerDistribution = []float64{0.25, 0.35, 0.40}
	cfg.Monitoring.Enabled = true
	cfg.Monitoring.MarketScanIntervalMinutes = 30
	analysis := agent1.MarketAnalysis{BTCPrice: 60000, MarketRegime: "ACCUMULATION", ActionPermission: agent1.Allowed, RiskLevel: agent1.Medium, PrimarySupportZone: market.Zone{Low: 58000, High: 59000}, InvalidationZone: market.Zone{Low: 56000, High: 57000}, ResistanceZone: market.Zone{Low: 65000, High: 66000}, BTCAccumulation: accumulation.Result{Phase: accumulation.PhaseConfirmed, Score: 80}}
	plan := agent2.Plan{State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed, Assets: []agent2.AssetPlan{
		{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, SetupScore: 0.90, Invalidation: 2800},
		{Symbol: "SOLUSDT", State: agent2.StateArmed, SetupScore: 0.80},
		{Symbol: "RENDERUSDT", State: agent2.StateWatch, SetupScore: 0.40},
	}, Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{{Symbol: "ETHUSDT", ReadinessScore: 0.90, NextTrigger: "reclaim support"}, {Symbol: "SOLUSDT", ReadinessScore: 0.80}}}}

	r := Build(cfg, analysis, plan)
	if r.Capital.ReserveCashUSDT != 100 {
		t.Fatalf("reserve=%v", r.Capital.ReserveCashUSDT)
	}
	if r.Capital.CycleDeploymentCapUSDT != 450 {
		t.Fatalf("cycle cap=%v", r.Capital.CycleDeploymentCapUSDT)
	}
	eth := findAsset(t, r, "ETHUSDT")
	if eth.ExecutableBudgetUSDT <= 0 || len(eth.LayerBudgetsUSDT) != 3 {
		t.Fatalf("ETH should be executable with layers: %+v", eth)
	}
	sol := findAsset(t, r, "SOLUSDT")
	if sol.ExecutableBudgetUSDT != 0 || sol.OpportunityBudgetUSDT <= 0 {
		t.Fatalf("ARMED SOL should only hold opportunity budget: %+v", sol)
	}
}

func TestFingerprintIgnoresPriceOnlyChange(t *testing.T) {
	cfg, analysis, plan := smallFixture()
	a := Build(cfg, analysis, plan)
	analysis.BTCPrice += 123.45
	b := Build(cfg, analysis, plan)
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("price-only change should not change semantic fingerprint: %s %s", a.Fingerprint, b.Fingerprint)
	}
	plan.State = agent2.StateArmed
	c := Build(cfg, analysis, plan)
	if a.Fingerprint == c.Fingerprint {
		t.Fatal("plan-state change must change fingerprint")
	}
}

func TestTelegramDigestContainsCapitalAndTriggers(t *testing.T) {
	cfg, analysis, plan := smallFixture()
	text := TelegramDigest(Build(cfg, analysis, plan))
	for _, want := range []string{"PHÂN BỔ VỐN", "trigger:", "THEO DÕI", "không futures"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func smallFixture() (config.Config, agent1.MarketAnalysis, agent2.Plan) {
	var cfg config.Config
	cfg.App.Mode = "paper"
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.ReserveCashRatio = 0.05
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 1}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.5
	cfg.Risk.MaxSingleAssetDeployment = 0.5
	cfg.Execution.LayerDistribution = []float64{1}
	analysis := agent1.MarketAnalysis{BTCPrice: 60000, MarketRegime: "RANGE", ActionPermission: agent1.Watch, RiskLevel: agent1.Medium, BTCAccumulation: accumulation.Result{Phase: accumulation.PhaseMarkdown, Score: 35}}
	plan := agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.5, NextTrigger: "reclaim"}}}
	return cfg, analysis, plan
}

func findAsset(t *testing.T, r Report, symbol string) AssetCapitalPlan {
	t.Helper()
	for _, a := range r.Capital.Assets {
		if a.Symbol == symbol {
			return a
		}
	}
	t.Fatalf("missing %s", symbol)
	return AssetCapitalPlan{}
}

func TestBuildCapitalSubtractsLiveExposureBeforeAllocating(t *testing.T) {
	cfg, analysis, plan := smallFixture()
	cfg.Portfolio.ReserveCashRatio = 0.10
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 1}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.50
	cfg.Risk.MaxSingleAssetDeployment = 1
	analysis.ActionPermission = agent1.Allowed
	plan.ActionPermission = agent1.Allowed
	plan.State = agent2.StateActiveLimit
	plan.Assets[0].State = agent2.StateActiveLimit
	plan.Assets[0].SetupScore = 0.90
	exposure := ExposureSnapshot{
		PositionCostUSDT:      150,
		OpenOrderNotionalUSDT: 50,
		Assets: map[string]AssetExposure{
			"ETHUSDT": {PositionCostUSDT: 150, OpenOrderNotionalUSDT: 50},
		},
		Source: "test ledger",
	}

	r := Build(cfg, analysis, plan, exposure)
	if r.Capital.CycleDeploymentCapUSDT != 450 {
		t.Fatalf("cycle cap=%v", r.Capital.CycleDeploymentCapUSDT)
	}
	if r.Capital.AlreadyCommittedUSDT != 200 || r.Capital.AvailableCycleCapacityUSDT != 250 {
		t.Fatalf("unexpected exposure/capacity: %+v", r.Capital)
	}
	if r.Capital.ExecutableNowUSDT > r.Capital.AvailableCycleCapacityUSDT {
		t.Fatalf("executable %.2f exceeds available %.2f", r.Capital.ExecutableNowUSDT, r.Capital.AvailableCycleCapacityUSDT)
	}
	eth := findAsset(t, r, "ETHUSDT")
	if eth.ExistingExposureUSDT != 200 || eth.RemainingStrategicUSDT != 700 {
		t.Fatalf("unexpected ETH exposure: %+v", eth)
	}
	if r.Capital.OpportunityReservedUSDT > r.Capital.AvailableCycleCapacityUSDT {
		t.Fatalf("opportunity envelope exceeds available capacity: %+v", r.Capital)
	}
}

func TestFingerprintChangesOnMaterialExposureButNotNumericTriggerDrift(t *testing.T) {
	cfg, analysis, plan := smallFixture()
	plan.Assets[0].NextTrigger = "reclaim 2,800 USDT"
	a := Build(cfg, analysis, plan)
	plan.Assets[0].NextTrigger = "reclaim 2,850 USDT"
	b := Build(cfg, analysis, plan)
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("numeric trigger drift should not spam state alerts: %s %s", a.Fingerprint, b.Fingerprint)
	}
	exposure := ExposureSnapshot{
		PositionCostUSDT: 200,
		Assets: map[string]AssetExposure{
			"ETHUSDT": {PositionCostUSDT: 200},
		},
	}
	c := Build(cfg, analysis, plan, exposure)
	if b.Fingerprint == c.Fingerprint {
		t.Fatal("material capital exposure change must change semantic fingerprint")
	}
}
