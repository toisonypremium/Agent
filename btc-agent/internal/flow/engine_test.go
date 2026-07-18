package flow

import (
	"testing"
	"time"

	"btc-agent/internal/market"
)

func flowCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*7)%40)
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 1, Volume: 1000}
	}
	return out
}

func TestDetectSweepLowReclaim(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 100, High: 108, Low: 96, Close: 106, Volume: 2200}
	got := Analyze(c, "1d", 60)
	if !got.SweepLow || !got.ReclaimSupport {
		t.Fatalf("expected sweep low reclaim: %+v", got)
	}
	if got.FlowBias != BiasAccumulation && got.FlowBias != BiasBearTrap {
		t.Fatalf("expected accumulation/bear trap bias, got %+v", got)
	}
}

func TestDetectAbsorption(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 104, High: 108, Low: 96, Close: 106, Volume: 2200}
	got := Analyze(c, "1d", 60)
	if !got.Absorption {
		t.Fatalf("expected absorption: %+v", got)
	}
}

func TestDetectFailedBreakoutDistribution(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 137, High: 148, Low: 132, Close: 134, Volume: 2300}
	got := Analyze(c, "1d", 60)
	if !got.FailedBreakout || !got.Distribution {
		t.Fatalf("expected failed breakout distribution: %+v", got)
	}
	if got.FlowBias != BiasBullTrap && got.FlowBias != BiasDistribution {
		t.Fatalf("expected bearish flow bias: %+v", got)
	}
}

func TestAnalyzeMultiFrameAggregatesBias(t *testing.T) {
	daily := flowCandles(80)
	last := len(daily) - 1
	daily[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 104, High: 108, Low: 96, Close: 106, Volume: 2200}
	m := AnalyzeMultiFrame(map[string][]market.Candle{"1d": daily, "4h": flowCandles(80), "1w": flowCandles(80)})
	if m.Bias != BiasAccumulation && m.Bias != BiasBearTrap {
		t.Fatalf("expected bullish accumulation context: %+v", m)
	}
	if m.Score <= 0 {
		t.Fatalf("expected positive score: %+v", m)
	}
}

func TestAggregateBiasPreservesAlignedAccumulation(t *testing.T) {
	got := aggregateBias(
		Signal{Timeframe: "1d", FlowBias: BiasAccumulation, Confidence: 0.90},
		Signal{Timeframe: "4h", FlowBias: BiasAccumulation, Confidence: 0.90},
		Signal{Timeframe: "1w", FlowBias: BiasNeutral, Confidence: 0.50},
	)
	if got != BiasAccumulation {
		t.Fatalf("aligned accumulation must remain accumulation, got %s", got)
	}
}

func TestAggregateBiasDoesNotPromoteAccumulationAgainstBearishWeeklyContext(t *testing.T) {
	got := aggregateBias(
		Signal{Timeframe: "1d", FlowBias: BiasAccumulation, Confidence: 0.90},
		Signal{Timeframe: "4h", FlowBias: BiasAccumulation, Confidence: 0.90},
		Signal{Timeframe: "1w", FlowBias: BiasDistribution, Confidence: 0.90},
	)
	if got == BiasAccumulation || got == BiasBearTrap {
		t.Fatalf("bullish lower timeframes must not promote accumulation against bearish weekly context: %s", got)
	}
}

func TestAggregateBiasDoesNotPromoteAccumulationFromIntradayAgainstBearishWeekly(t *testing.T) {
	got := aggregateBias(
		Signal{Timeframe: "1d", FlowBias: BiasNeutral, Confidence: 0.10},
		Signal{Timeframe: "4h", FlowBias: BiasAccumulation, Confidence: 1.0},
		Signal{Timeframe: "1w", FlowBias: BiasDistribution, Confidence: 1.0},
	)
	if got == BiasAccumulation {
		t.Fatalf("intraday bullish weight must not promote accumulation against bearish weekly context: %s", got)
	}
}

func TestAnalyzeWithParamsDetectsBorderlineAbsorption(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 103, High: 108, Low: 96, Close: 105, Volume: 1220}
	strict := Params{VolumeHighMultiplier: 1.35, WickRatio: 0.35, NearSupportLow: 1.02, NearSupportClose: 1.03, NearResistanceHigh: 0.98, NearResistanceClose: 0.97, AccumulationScore: 0.35, DistributionScore: 0.40, TrapScore: 0.45}
	if AnalyzeWithParams(c, "1d", 60, strict).Absorption {
		t.Fatal("strict params should not detect borderline absorption")
	}
	got := AnalyzeWithParams(c, "1d", 60, DefaultParams())
	if !got.Absorption || got.BullScore <= 0 {
		t.Fatalf("default tuned params should detect borderline absorption with bull score: %+v", got)
	}
}

func TestNormalVolumeSmallWickDoesNotTriggerFlow(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 104, High: 106, Low: 101, Close: 105, Volume: 1000}
	got := Analyze(c, "1d", 60)
	if got.Absorption || got.Distribution || got.FlowBias != BiasNeutral {
		t.Fatalf("normal candle should remain neutral: %+v", got)
	}
}

func TestFlowComponentsAndDiagnostics(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 100, High: 108, Low: 96, Close: 106, Volume: 2200}
	got := Analyze(c, "1d", 60)
	if len(got.Components) == 0 {
		t.Fatalf("expected flow components: %+v", got)
	}
	if got.Diagnostics.LastVolume <= 0 || got.Diagnostics.AvgVolume <= 0 {
		t.Fatalf("expected volume diagnostics: %+v", got.Diagnostics)
	}
	found := false
	for _, c := range got.Components {
		if c.Name == "reclaim_support" && c.Pass && c.Bull > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected reclaim_support component: %+v", got.Components)
	}
}

func TestNeutralFlowHasNextTrigger(t *testing.T) {
	c := flowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 104, High: 106, Low: 101, Close: 105, Volume: 1000}
	got := Analyze(c, "1d", 60)
	if got.FlowBias != BiasNeutral || got.Diagnostics.NextBullTrigger == "" {
		t.Fatalf("expected neutral next trigger: %+v", got)
	}
}
