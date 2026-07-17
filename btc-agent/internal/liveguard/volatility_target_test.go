package liveguard

import (
	"btc-agent/internal/market"
	"math"
	"testing"
	"time"
)

func volCandles(rs []float64) []market.Candle {
	out := make([]market.Candle, len(rs)+1)
	p := 100.0
	for i := range out {
		if i > 0 {
			p *= math.Exp(rs[i-1])
		}
		out[i] = market.Candle{OpenTime: time.Unix(int64(i*86400), 0), Close: p}
	}
	return out
}
func TestVolatilityTargetReducesHighVol(t *testing.T) {
	rs := []float64{}
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			rs = append(rs, .05)
		} else {
			rs = append(rs, -.05)
		}
	}
	got := EvaluateVolatilityTarget(VolatilityTargetInput{RequestedNotional: 100, Candles: volCandles(rs), TargetAnnualVol: .40, MinMultiplier: .25, MaxMultiplier: 1, MinObservations: 45})
	if !got.Allowed || got.AdjustedNotional >= 100 || got.Multiplier < .25 {
		t.Fatalf("bad reduction: %+v", got)
	}
}
func TestVolatilityTargetNeverUpsizes(t *testing.T) {
	rs := make([]float64, 60)
	for i := range rs {
		if i%2 == 0 {
			rs[i] = .001
		} else {
			rs[i] = -.001
		}
	}
	got := EvaluateVolatilityTarget(VolatilityTargetInput{RequestedNotional: 100, Candles: volCandles(rs), TargetAnnualVol: .40, MinMultiplier: .25, MaxMultiplier: 1, MinObservations: 45})
	if !got.Allowed || got.AdjustedNotional != 100 || got.Multiplier != 1 {
		t.Fatalf("unexpected upsize: %+v", got)
	}
}
func TestVolatilityTargetFailsClosedOnShortHistory(t *testing.T) {
	got := EvaluateVolatilityTarget(VolatilityTargetInput{RequestedNotional: 100, Candles: volCandles([]float64{.01, -.01}), TargetAnnualVol: .4, MinObservations: 45})
	if got.Allowed {
		t.Fatalf("expected closed: %+v", got)
	}
}
