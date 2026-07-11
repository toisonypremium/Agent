package accumulation

import (
	"testing"
	"time"

	"btc-agent/internal/market"
)

func accCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*3)%20)
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 0.5, Volume: 1000}
	}
	return out
}

func TestAnalyzeFallingKnifeInvalidated(t *testing.T) {
	c := accCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 95, High: 96, Low: 80, Close: 82, Volume: 3000}
	got := Analyze("BTCUSDT", c)
	if got.Phase != PhaseInvalidated {
		t.Fatalf("expected invalidated falling knife: %+v", got)
	}
	if len(got.HardBlockers) == 0 {
		t.Fatalf("expected hard blocker: %+v", got)
	}
}

func TestAnalyzeSweepWaitsForReclaim(t *testing.T) {
	c := accCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 99, High: 101, Low: 88, Close: 97, Volume: 900}
	got := Analyze("BTCUSDT", c)
	if got.Phase != PhaseSweep {
		t.Fatalf("expected sweep: %+v", got)
	}
}

func TestAnalyzeReclaimOrConfirmed(t *testing.T) {
	c := accCandles(80)
	last := len(c) - 1
	c[last-2] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 96, High: 106, Low: 88, Close: 104, Volume: 2600}
	c[last-1] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 104, High: 108, Low: 99, Close: 103, Volume: 900}
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 103, High: 109, Low: 87, Close: 106, Volume: 2600}
	got := Analyze("BTCUSDT", c)
	if got.Phase != PhaseReclaim && got.Phase != PhaseConfirmed {
		t.Fatalf("expected reclaim/confirmed: %+v", got)
	}
	if got.Score < 60 {
		t.Fatalf("expected useful score: %+v", got)
	}
}

func TestAnalyzeDistributionHardBlocks(t *testing.T) {
	c := accCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", Open: 118, High: 145, Low: 115, Close: 116, Volume: 2800}
	got := Analyze("BTCUSDT", c)
	if got.Phase != PhaseDistribution {
		t.Fatalf("expected distribution: %+v", got)
	}
	if len(got.HardBlockers) == 0 {
		t.Fatalf("expected hard blocker: %+v", got)
	}
}

func TestAnalyzeLowDataDoesNotConfirm(t *testing.T) {
	got := Analyze("BTCUSDT", accCandles(10))
	if got.Phase == PhaseConfirmed || got.DataQuality >= 0.6 {
		t.Fatalf("low data must not confirm: %+v", got)
	}
}
