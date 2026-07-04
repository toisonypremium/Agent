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

func TestBuildPlanRequiresAgent1Allowed(t *testing.T) {
	analysis := allowedAnalysis()
	analysis.ActionPermission = agent1.Watch
	got := BuildPlan(testConfig(), analysis, map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)})
	if got.State == StateActiveLimit || len(got.Assets) != 0 {
		t.Fatalf("WATCH permission must not create active plan: %+v", got)
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

func TestBuildPlanWithBenchmarksBlocksWeakRelativeStrength(t *testing.T) {
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
	if len(got.Assets) == 0 || got.Assets[0].State != StateNoTrade || !strings.Contains(got.Assets[0].Reason, "relative strength filter") {
		t.Fatalf("expected relative strength block: %+v", got)
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
	if len(got.Assets) == 0 || got.Assets[0].State != StateWatch || !strings.Contains(got.Assets[0].Reason, "asset flow entry") {
		t.Fatalf("expected neutral flow entry watch: %+v", got)
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

func TestPaperOrdersFromPlanOnlyActiveLimit(t *testing.T) {
	p := Plan{Assets: []AssetPlan{{Symbol: "ETHUSDT", State: StateWatch}, {Symbol: "SOLUSDT", State: StateActiveLimit, Invalidation: 90, Layers: []Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}}}}}
	orders := OrdersFromPlan(p, 48)
	if len(orders) != 1 || orders[0].Symbol != "SOLUSDT" {
		t.Fatalf("unexpected orders: %+v", orders)
	}
}

func TestFallingKnifeFilterBlocksLowerLows(t *testing.T) {
	candles := assetCandles(10, false)
	for i := 6; i < 10; i++ {
		candles[i].Low = candles[i-1].Low - 5
		candles[i].Close = candles[i].Low + 1
	}
	if !FallingKnife(candles) {
		t.Fatal("expected falling knife")
	}
}

func TestFOMOFilterBlocksHotExtension(t *testing.T) {
	candles := assetCandles(10, false)
	for i := 6; i < 10; i++ {
		candles[i].Open = float64(i) * 10
		candles[i].Close = candles[i].Open + 20
	}
	if !FOMO(candles, 100, 50, market.Zone{}) {
		t.Fatal("expected FOMO block")
	}
}
