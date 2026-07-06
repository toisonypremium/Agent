package agent2

import (
	"testing"
	"time"

	"btc-agent/internal/market"
)

func mmCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*3)%20)
		out[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 0.5, Volume: 1000}
	}
	return out
}

func TestAnalyzeMMAccumulationFallingKnife(t *testing.T) {
	c := mmCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 95, High: 96, Low: 80, Close: 82, Volume: 3000}
	got := AnalyzeMMAccumulation("ETHUSDT", c)
	if got.Case != MMCaseFallingKnife || !got.HardBlock {
		t.Fatalf("expected falling knife: %+v", got)
	}
}

func TestAnalyzeMMAccumulationFailedSweep(t *testing.T) {
	c := mmCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 99, High: 101, Low: 88, Close: 97, Volume: 900}
	got := AnalyzeMMAccumulation("ETHUSDT", c)
	if got.Case != MMCaseFailedSweep {
		t.Fatalf("expected failed sweep: %+v", got)
	}
}

func TestAnalyzeMMAccumulationSpringReclaim(t *testing.T) {
	c := mmCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 97, High: 108, Low: 88, Close: 104, Volume: 2600}
	got := AnalyzeMMAccumulation("ETHUSDT", c)
	if got.Case != MMCaseSpringReclaim && got.Case != MMCaseArmedProbeCandidate {
		t.Fatalf("expected spring/reclaim: %+v", got)
	}
	if !got.Pass {
		t.Fatalf("expected pass: %+v", got)
	}
}

func TestAnalyzeMMAccumulationDistributionTrap(t *testing.T) {
	c := mmCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 118, High: 145, Low: 115, Close: 116, Volume: 2800}
	got := AnalyzeMMAccumulation("ETHUSDT", c)
	if got.Case != MMCaseDistributionTrap || !got.HardBlock {
		t.Fatalf("expected distribution trap: %+v", got)
	}
}
