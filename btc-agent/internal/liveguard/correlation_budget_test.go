package liveguard

import (
	"btc-agent/internal/market"
	"math"
	"testing"
	"time"
)

func corrCandles(sym string, rs []float64, offset int) []market.Candle {
	out := make([]market.Candle, len(rs)+1)
	p := 100.0
	for i := range out {
		if i > 0 {
			p *= 1 + rs[i-1]
		}
		out[i] = market.Candle{Symbol: sym, OpenTime: time.Unix(int64((i+offset)*86400), 0), Close: p}
	}
	return out
}
func TestAlignedReturnCorrelation(t *testing.T) {
	r := []float64{.01, -.02, .03, .01, -.01}
	a := corrCandles("A", r, 0)
	b := corrCandles("B", r, 0)
	c, n := AlignedReturnCorrelation(a, b)
	if n != 5 || math.Abs(c-1) > .000001 {
		t.Fatalf("corr=%f n=%d", c, n)
	}
}
func TestCorrelationBudgetClampsCluster(t *testing.T) {
	r := []float64{.01, -.02, .03, .01, -.01, .02, -.01, .03, .01, -.02}
	cs := map[string][]market.Candle{"A": corrCandles("A", r, 0), "B": corrCandles("B", r, 0)}
	got := EvaluateCorrelationBudget(CorrelationBudgetInput{Symbol: "A", RequestedNotional: 30, ExistingExposure: map[string]float64{"B": 80}, Candles: cs, Threshold: .75, ClusterCap: 100, MinObservations: 10})
	if !got.Allowed || got.AdjustedNotional != 20 || got.CorrelationExposure != 80 {
		t.Fatalf("bad cluster budget: %+v", got)
	}
}
func TestCorrelationBudgetFailsClosedOnInsufficientOverlap(t *testing.T) {
	r := []float64{.01, -.02, .03}
	got := EvaluateCorrelationBudget(CorrelationBudgetInput{Symbol: "A", RequestedNotional: 10, ExistingExposure: map[string]float64{"B": 10}, Candles: map[string][]market.Candle{"A": corrCandles("A", r, 0), "B": corrCandles("B", r, 20)}, Threshold: .75, ClusterCap: 100, MinObservations: 10})
	if got.Allowed {
		t.Fatalf("expected closed: %+v", got)
	}
}
