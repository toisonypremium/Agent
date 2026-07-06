package agent1

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func testConfig() config.Config {
	var cfg config.Config
	cfg.App.Mode = "paper"
	cfg.Storage.Path = "data/test.db"
	cfg.Portfolio.BaseCurrency = "USDT"
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": .35, "SOLUSDT": .45, "RENDERUSDT": .20}
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	cfg.Risk.MinRewardRisk = 3
	cfg.Execution.PaperTrading = true
	cfg.Execution.LayerDistribution = []float64{.25, .35, .40}
	cfg.Data.BinanceBaseURL = "https://api.binance.com"
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	cfg.Data.Intervals = []string{"4h", "1d", "1w"}
	cfg.Data.CandleLimit = 120
	cfg.BTCCycle.StressPriceReference = 28000
	return cfg
}

func trendCandles(n int, start, step float64) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	price := start
	for i := range out {
		open := price
		close := price + step
		high := max(open, close) * 1.01
		low := min(open, close) * .99
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: open, High: high, Low: low, Close: close, Volume: 1000}
		price = close
	}
	return out
}

func btcFrames(c []market.Candle) map[string][]market.Candle {
	return map[string][]market.Candle{"4h": c, "1d": c, "1w": c}
}

func TestAnalyzePanicSellingForcesNoTrade(t *testing.T) {
	candles := trendCandles(120, 100000, -500)
	for i := 115; i < 120; i++ {
		candles[i].Close = candles[i-1].Low * .96
		candles[i].Low = candles[i].Close * .98
		candles[i].High = candles[i].Open * 1.01
		candles[i].Volume = candles[i-1].Volume * 1.8
	}
	got, err := Analyze(testConfig(), btcFrames(candles), exchange.FearGreed{Value: 10, Classification: "Extreme Fear"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActionPermission != NoTrade {
		t.Fatalf("panic/downtrend must force NO_TRADE, got %+v", got)
	}
	if got.FallingKnifeRisk != High && got.MarketRegime != "PANIC_SELLING" {
		t.Fatalf("expected panic or high falling knife risk, got regime=%s falling=%s", got.MarketRegime, got.FallingKnifeRisk)
	}
}

func TestAnalyzeDowntrendDoesNotAllowTrading(t *testing.T) {
	got, err := Analyze(testConfig(), btcFrames(trendCandles(120, 100000, -250)), exchange.FearGreed{Value: 30, Classification: "Fear"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActionPermission == Allowed {
		t.Fatalf("downtrend must not be ALLOWED: %+v", got)
	}
}

func TestAnalyzeHighFOMORiskForcesNoTrade(t *testing.T) {
	candles := trendCandles(120, 50000, 400)
	for i := 116; i < 120; i++ {
		candles[i].Open = candles[i-1].Close
		candles[i].Close = candles[i].Open * 1.04
		candles[i].High = candles[i].Close * 1.02
		candles[i].Low = candles[i].Open * .995
	}
	got, err := Analyze(testConfig(), btcFrames(candles), exchange.FearGreed{Value: 85, Classification: "Extreme Greed"})
	if err != nil {
		t.Fatal(err)
	}
	if got.FomoRisk != High || got.ActionPermission != NoTrade {
		t.Fatalf("high FOMO must force NO_TRADE, got fomo=%s perm=%s", got.FomoRisk, got.ActionPermission)
	}
}

func TestAnalyzeHasClearZones(t *testing.T) {
	got, err := Analyze(testConfig(), btcFrames(trendCandles(120, 50000, 50)), exchange.FearGreed{Value: 45, Classification: "Neutral"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.PrimarySupportZone.Valid() || !got.ResistanceZone.Valid() || !got.InvalidationZone.Valid() {
		t.Fatalf("expected valid zones: %+v", got)
	}
}

func TestAnalyzeBullTrapFlowBlocksAllowed(t *testing.T) {
	candles := trendCandles(120, 50000, 120)
	last := len(candles) - 1
	candles[last].Open = 64500
	candles[last].High = 65000
	candles[last].Low = 61000
	candles[last].Close = 61200
	candles[last].Volume = 3000
	got, err := Analyze(testConfig(), btcFrames(candles), exchange.FearGreed{Value: 55, Classification: "Neutral"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Flow.Bias != flow.BiasBullTrap && got.Flow.Bias != flow.BiasDistribution {
		t.Fatalf("expected trap/distribution flow: %+v", got.Flow)
	}
	if got.ActionPermission == Allowed {
		t.Fatalf("bull trap flow must not be ALLOWED: %+v", got)
	}
}

func TestAnalyzeFlowAppearsInJSONAndReport(t *testing.T) {
	candles := trendCandles(120, 50000, 80)
	last := len(candles) - 1
	candles[last].Open = 58500
	candles[last].High = 59500
	candles[last].Low = 57500
	candles[last].Close = 59200
	candles[last].Volume = 2500
	got, err := Analyze(testConfig(), btcFrames(candles), exchange.FearGreed{Value: 40, Classification: "Neutral"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Flow.Bias == "" {
		t.Fatalf("expected flow field: %+v", got.Flow)
	}
	if !strings.Contains(got.JSON(), "\"flow\"") || !strings.Contains(got.JSON(), "\"score_breakdown\"") || got.PermissionReason == "" {
		t.Fatal("expected flow, score breakdown, and permission reason in JSON")
	}
	if !strings.Contains(DailyReport(got, "test plan"), "MM / Liquidity Flow") {
		t.Fatal("expected flow section in report")
	}
}
