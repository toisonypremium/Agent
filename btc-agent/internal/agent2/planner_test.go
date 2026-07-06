package agent2

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func testConfig() config.Config {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": .35, "SOLUSDT": .45, "RENDERUSDT": .20}
	cfg.Risk.MaxTotalDeploymentPerCycle = .70
	cfg.Risk.MaxSingleAssetDeployment = .45
	cfg.Risk.MinRewardRisk = 3
	cfg.Execution.LayerDistribution = []float64{.25, .35, .40}
	cfg.Execution.OrderExpiryHours = 48
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	return cfg
}

func allowedAnalysis() agent1.MarketAnalysis {
	return agent1.MarketAnalysis{ActionPermission: agent1.Allowed, MarketRegime: "ACCUMULATION", FallingKnifeRisk: agent1.Low, FomoRisk: agent1.Low}
}

func assetCandles(n int, lastNearSupport bool) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*7)%50)
		if lastNearSupport && i > n-5 {
			price = 100 + float64(i-n+5)*0.5
		}
		out[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price * 1.005, High: price * 1.02, Low: price * .98, Close: price, Volume: 1000}
	}
	return out
}

func TestBuildPlanTargetsConfiguredThreeCoinsAndExcludesBTC(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.BTC = "BTCUSDT"
	assets := map[string][]market.Candle{
		"BTCUSDT":    assetCandles(80, true),
		"ETHUSDT":    assetCandles(80, true),
		"SOLUSDT":    assetCandles(80, true),
		"RENDERUSDT": assetCandles(80, true),
		"DOGEUSDT":   assetCandles(80, true),
	}
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), assets, map[string][]market.Candle{"BTCUSDT": assetCandles(80, false)})
	if len(got.Assets) != 3 {
		t.Fatalf("expected 3 configured asset plans, got %+v", got.Assets)
	}
	for _, asset := range got.Assets {
		if asset.Symbol == "BTCUSDT" || asset.Symbol == "DOGEUSDT" {
			t.Fatalf("unexpected non-target asset in plan: %+v", got.Assets)
		}
	}
	if len(got.Watchlist.Candidates) != 3 {
		t.Fatalf("expected 3 configured watch candidates, got %+v", got.Watchlist.Candidates)
	}
	if len(got.Rotation) != 3 {
		t.Fatalf("expected 3 configured rotation rows, got %+v", got.Rotation)
	}
}

func TestBuildPlanRequiresAgent1Allowed(t *testing.T) {
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	got := BuildPlan(testConfig(), analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)})
	if got.State == StateActiveLimit {
		t.Fatalf("WATCH permission must not create full active plan: %+v", got)
	}
}

func TestBuildPlanBlocksHighRisk(t *testing.T) {
	analysis := allowedAnalysis()
	analysis.FallingKnifeRisk = agent1.High
	got := BuildPlan(testConfig(), analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)})
	if got.State == StateActiveLimit {
		t.Fatalf("high risk must block active limit: %+v", got)
	}
}

func TestBuildPlanCreatesThreeLayersWhenAllowed(t *testing.T) {
	cfg := testConfig()
	got := BuildPlan(cfg, allowedAnalysis(), map[string][]market.Candle{"ETHUSDT": assetCandles(80, true), "SOLUSDT": nil, "RENDERUSDT": nil})
	if len(got.Assets) == 0 || got.Assets[0].State != StateActiveLimit {
		t.Fatalf("expected active limit for ETH: %+v", got)
	}
	asset := got.Assets[0]
	if len(asset.Layers) != 3 {
		t.Fatalf("expected 3 layers: %+v", asset.Layers)
	}
	for i, want := range []float64{.25, .35, .40} {
		if asset.Layers[i].Fraction != want {
			t.Fatalf("layer %d fraction=%v want %v", i, asset.Layers[i].Fraction, want)
		}
	}
	total := 0.0
	for _, layer := range asset.Layers {
		total += layer.Notional
	}
	maxBudget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation["ETHUSDT"] * cfg.Risk.MaxTotalDeploymentPerCycle
	if total > maxBudget+1e-9 {
		t.Fatalf("notional %v above budget %v", total, maxBudget)
	}
}

func TestBuildPlanWithBenchmarksSoftWaitsWeakRelativeStrength(t *testing.T) {
	cfg := testConfig()
	asset := assetCandles(80, true)
	btc := assetCandles(80, false)
	for i := len(asset) - 15; i < len(asset); i++ {
		step := i - (len(asset) - 15)
		assetPrice := 120 - float64(step)*1.5
		if step >= 10 {
			assetPrice = 100 + float64(step-10)*0.2
		}
		asset[i].Close = assetPrice
		asset[i].Open = asset[i].Close * 1.005
		asset[i].High = asset[i].Close * 1.01
		asset[i].Low = asset[i].Close * 0.99
		btcPrice := 100 - float64(step)*0.15
		btc[i].Close = btcPrice
		btc[i].Open = btc[i].Close
		btc[i].High = btc[i].Close * 1.01
		btc[i].Low = btc[i].Close * 0.99
	}
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), map[string][]market.Candle{"ETHUSDT": asset, "SOLUSDT": nil, "RENDERUSDT": nil}, map[string][]market.Candle{"BTCUSDT": btc})
	if len(got.Assets) == 0 || got.Assets[0].State == StateNoTrade || !strings.Contains(got.Assets[0].Reason, "relative strength filter") {
		t.Fatalf("expected relative strength soft wait, not hard block: %+v", got)
	}
}

func TestBuildPlanWithBenchmarksIncludesRotationRanking(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.MinRotationScore = 0.1
	btc := scoreCandles("BTCUSDT", 100, 102, 80)
	assets := map[string][]market.Candle{
		"ETHUSDT":    scoreCandles("ETHUSDT", 100, 112, 80),
		"SOLUSDT":    scoreCandles("SOLUSDT", 100, 105, 80),
		"RENDERUSDT": scoreCandles("RENDERUSDT", 100, 103, 80),
	}
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), assets, map[string][]market.Candle{"BTCUSDT": btc})
	if len(got.Rotation) != 3 {
		t.Fatalf("expected 3 ranking rows: %+v", got.Rotation)
	}
	if got.Rotation[0].Rank != 1 || got.Rotation[1].Rank != 2 || got.Rotation[2].Rank != 3 {
		t.Fatalf("ranking not sorted: %+v", got.Rotation)
	}
}

func TestBuildPlanWithBenchmarksBlocksLowRotationRank(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.MinRotationScore = 0.1
	cfg.Risk.MaxRotationRank = 2
	btc := assetCandles(80, false)
	assets := map[string][]market.Candle{
		"ETHUSDT":    assetCandles(80, true),
		"SOLUSDT":    assetCandles(80, true),
		"RENDERUSDT": assetCandles(80, true),
	}
	for i := 65; i < 80; i++ {
		step := float64(i - 65)
		btc[i].Close = 100 + step*0.05
		btc[i].Open = btc[i].Close * 1.002
		btc[i].High = btc[i].Close * 1.015
		btc[i].Low = btc[i].Close * 0.985
		assets["ETHUSDT"][i].Close = 96 + step*0.30
		assets["SOLUSDT"][i].Close = 98 + step*0.18
		assets["RENDERUSDT"][i].Close = 100 + float64((i%3)-1)*0.15
		for _, sym := range []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"} {
			c := &assets[sym][i]
			c.Open = c.Close * 1.002
			c.High = c.Close * 1.015
			c.Low = c.Close * 0.985
		}
	}
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), assets, map[string][]market.Candle{"BTCUSDT": btc})
	var render AssetPlan
	for _, asset := range got.Assets {
		if asset.Symbol == "RENDERUSDT" {
			render = asset
		}
	}
	if render.Symbol == "" || render.State != StateWatch || render.RotationRank != 3 || !strings.Contains(render.Reason, "rotation score filter") {
		t.Fatalf("expected rank 3 rotation block: %+v", got)
	}
}

func TestBuildPlanWithBenchmarksBlocksNeutralAssetFlowEntry(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.MinRewardRisk = 0.1
	asset := assetCandles(80, true)
	btc := assetCandles(80, false)
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), map[string][]market.Candle{"ETHUSDT": asset, "SOLUSDT": nil, "RENDERUSDT": nil}, map[string][]market.Candle{"BTCUSDT": btc})
	if len(got.Assets) == 0 || got.Assets[0].State != StateWatch || !hasReason(got.Assets[0].Reasons, ReasonAssetFlowEntry, ReasonSoftWait) {
		t.Fatalf("expected neutral flow entry soft-wait watch: %+v", got)
	}
}

func TestBuildPlanWithBenchmarksAllowsBullishAssetFlowEntry(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.MinRewardRisk = 0.1
	asset := assetCandles(80, true)
	btc := assetCandles(80, false)
	last := len(asset) - 1
	asset[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 100, High: 108, Low: 96, Close: 101, Volume: 2200}
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), map[string][]market.Candle{"ETHUSDT": asset, "SOLUSDT": nil, "RENDERUSDT": nil}, map[string][]market.Candle{"BTCUSDT": btc})
	if len(got.Assets) == 0 || got.Assets[0].Reason == "" || strings.Contains(got.Assets[0].Reason, "asset flow entry chưa") {
		t.Fatalf("bullish flow should pass flow gate: %+v", got)
	}
}

func TestBuildPlanWatchPermissionCreatesScoutNoOrders(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	got := BuildPlanWithBenchmarks(cfg, analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}, nil)
	if got.State == StateArmed || got.State == StateActiveLimit {
		t.Fatalf("WATCH must not create ARMED/ACTIVE plan: %+v", got)
	}
	if len(got.Assets) != 1 || got.Assets[0].State != StateScout || len(got.Assets[0].SoftBlockers) == 0 || len(got.Assets[0].HardBlockers) != 0 {
		t.Fatalf("expected WATCH soft-gated scout asset: %+v", got)
	}
	if !hasReason(got.Assets[0].Reasons, ReasonBTCPermission, ReasonSoftWait) {
		t.Fatalf("expected typed BTC permission soft wait: %+v", got.Assets[0].Reasons)
	}
	if orders := OrdersFromPlan(got, 48); len(orders) != 0 {
		t.Fatalf("SCOUT must not create orders: %+v", orders)
	}
}

func TestBuildPlanDowntrendNoPanicAllowsScout(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	analysis := allowedAnalysis()
	analysis.MarketRegime = "DOWNTREND"
	analysis.ActionPermission = agent1.Watch
	analysis.FallingKnifeRisk = agent1.Low
	analysis.FomoRisk = agent1.Low
	got := BuildPlanWithBenchmarks(cfg, analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}, nil)
	if got.State != StateScout || len(got.Assets) != 1 || got.Assets[0].State != StateScout {
		t.Fatalf("DOWNTREND without panic should allow scout: %+v", got)
	}
	if !hasReason(got.Assets[0].Reasons, ReasonBTCDowntrend, ReasonSoftWait) {
		t.Fatalf("expected BTC downtrend soft wait: %+v", got.Assets[0].Reasons)
	}
	if orders := OrdersFromPlan(got, 48); len(orders) != 0 {
		t.Fatalf("DOWNTREND scout must not create orders: %+v", orders)
	}
}

func TestPaperOrdersFromPlanSkipsScoutAndArmed(t *testing.T) {
	p := Plan{Assets: []AssetPlan{
		{Symbol: "ETHUSDT", State: StateScout, Invalidation: 90, Layers: []Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}}},
		{Symbol: "RENDERUSDT", State: StateArmed, Invalidation: 90, Layers: []Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}}},
		{Symbol: "SOLUSDT", State: StateActiveLimit, Invalidation: 90, Layers: []Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}}},
	}}
	orders := OrdersFromPlan(p, 48)
	if len(orders) != 1 || orders[0].Symbol != "SOLUSDT" {
		t.Fatalf("only ACTIVE_LIMIT should create orders: %+v", orders)
	}
}

func hasReason(reasons []DecisionReason, code ReasonCode, severity ReasonSeverity) bool {
	for _, reason := range reasons {
		if reason.Code == code && reason.Severity == severity {
			return true
		}
	}
	return false
}

func TestBuildPlanLayersIncludeAuditFields(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	cfg.Risk.MinRewardRisk = 0.1
	got := BuildPlanWithBenchmarks(cfg, allowedAnalysis(), map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}, nil)
	if len(got.Assets) != 1 || len(got.Assets[0].Layers) == 0 {
		t.Fatalf("expected layers: %+v", got)
	}
	for _, layer := range got.Assets[0].Layers {
		if layer.Price <= 0 || layer.Notional <= 0 || layer.Quantity <= 0 || layer.Invalidation <= 0 || layer.Target <= 0 || layer.RewardRisk <= 0 || layer.Reason == "" || layer.ExpiresAt.IsZero() {
			t.Fatalf("layer missing audit fields: %+v", layer)
		}
	}
}

func TestPaperOrdersFromPlanOnlyActiveLimit(t *testing.T) {
	p := Plan{Assets: []AssetPlan{{Symbol: "ETHUSDT", State: StateWatch}, {Symbol: "SOLUSDT", State: StateActiveLimit, Invalidation: 90, Layers: []Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}}}}}
	orders := OrdersFromPlan(p, 48)
	if len(orders) != 1 || orders[0].Symbol != "SOLUSDT" {
		t.Fatalf("unexpected orders: %+v", orders)
	}
}

func TestFallingKnifeClassifierSoftLowerLows(t *testing.T) {
	candles := assetCandles(10, false)
	for i := 6; i < 10; i++ {
		candles[i].Low = candles[i-1].Low - 5
		candles[i].Close = candles[i].Low + 1
	}
	got := ClassifyAssetRisk(candles, 0, 0, market.Zone{})
	if got.FallingKnife != ReasonSoftWait {
		t.Fatalf("expected soft falling knife wait: %+v", got)
	}
	if FallingKnife(candles) {
		t.Fatal("mild lower lows should not hard block")
	}
}

func TestFallingKnifeClassifierHardBreakdownVolume(t *testing.T) {
	candles := assetCandles(10, false)
	last := len(candles) - 1
	candles[last].Close = candles[last-1].Low - 5
	candles[last].Low = candles[last].Close - 2
	candles[last].High = candles[last].Close + 1
	candles[last].Volume = candles[last-1].Volume * 2
	if !FallingKnife(candles) {
		t.Fatal("expected confirmed falling knife hard block")
	}
}

func TestFOMOClassifierSoftGreenCandles(t *testing.T) {
	candles := assetCandles(10, false)
	for i := 6; i < 10; i++ {
		candles[i].Open = 100
		candles[i].Close = 101
	}
	got := ClassifyAssetRisk(candles, 100, 50, market.Zone{})
	if got.FOMO != ReasonSoftWait {
		t.Fatalf("expected soft FOMO wait: %+v", got)
	}
}

func TestFOMOFilterBlocksHotExtension(t *testing.T) {
	candles := assetCandles(10, false)
	for i := 6; i < 10; i++ {
		candles[i].Open = float64(i) * 10
		candles[i].Close = candles[i].Open + 20
	}
	if !FOMO(candles, 100, 75, market.Zone{Low: 100, High: 120}) {
		t.Fatal("expected FOMO block")
	}
}

func TestBuildPlanCreatesProbeCandidateWhenBTCArmedAndAssetSetupStrong(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 2
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Armed
	got := BuildPlanWithBenchmarks(cfg, analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}, nil)
	if got.State != StateArmed || len(got.Assets) != 1 || got.Assets[0].State != StateArmed {
		t.Fatalf("expected ARMED probe plan: %+v", got)
	}
	if len(got.Assets[0].Layers) != 1 {
		t.Fatalf("expected one probe layer: %+v", got.Assets[0].Layers)
	}
}

func TestBuildPlanDoesNotProbeWhenBTCHardRisk(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Armed
	analysis.MarketRegime = "PANIC_SELLING"
	got := BuildPlanWithBenchmarks(cfg, analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}, nil)
	if got.State == StateArmed || got.State == StateActiveLimit || len(got.Assets) != 1 || got.Assets[0].State != StateNoTrade {
		t.Fatalf("hard BTC risk must not create probe: %+v", got)
	}
	if orders := OrdersFromPlan(got, 48); len(orders) != 0 {
		t.Fatalf("hard BTC risk must not create orders: %+v", orders)
	}
}

func TestBuildPlanDoesNotProbeWeakAsset(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.DisableRotationScoreFilter = true
	cfg.Risk.DisableAssetFlowEntryFilter = true
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Armed
	asset := assetCandles(80, true)
	for i := len(asset) - 5; i < len(asset); i++ {
		asset[i].Close = 150
		asset[i].Open = 150
		asset[i].High = 153
		asset[i].Low = 147
	}
	got := BuildPlanWithBenchmarks(cfg, analysis, map[string][]market.Candle{"ETHUSDT": asset}, nil)
	if got.State == StateArmed {
		t.Fatalf("weak/far asset must not create probe: %+v", got)
	}
}
