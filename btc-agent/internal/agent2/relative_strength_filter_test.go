package agent2

import (
	"testing"
	"time"

	"btc-agent/internal/market"
)

func rsCandles(n int, start, end float64) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		p := start
		if n > 1 {
			p = start + (end-start)*float64(i)/float64(n-1)
		}
		out[i] = market.Candle{OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: p, High: p * 1.01, Low: p * 0.99, Close: p, Volume: 1000}
	}
	return out
}

func TestRelativeStrengthBlocksWeakAsset(t *testing.T) {
	asset := rsCandles(20, 100, 88)
	btc := rsCandles(20, 100, 98)
	got := RelativeStrength(asset, btc, 14, -0.03, -0.05)
	if got.Pass {
		t.Fatalf("expected weak asset blocked: %+v", got)
	}
}

func TestRelativeStrengthAllowsSmallUnderperformance(t *testing.T) {
	asset := rsCandles(20, 100, 97)
	btc := rsCandles(20, 100, 98)
	got := RelativeStrength(asset, btc, 14, -0.03, -0.05)
	if !got.Pass {
		t.Fatalf("expected small underperformance allowed: %+v", got)
	}
}

func TestRelativeStrengthAllowsStrongAsset(t *testing.T) {
	asset := rsCandles(20, 100, 105)
	btc := rsCandles(20, 100, 98)
	got := RelativeStrength(asset, btc, 14, -0.03, -0.05)
	if !got.Pass {
		t.Fatalf("expected strong asset allowed: %+v", got)
	}
}
