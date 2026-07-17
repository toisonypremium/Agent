package liveguard

import (
	"btc-agent/internal/market"
	"fmt"
	"math"
)

type VolatilityTargetInput struct {
	RequestedNotional float64
	Candles           []market.Candle
	TargetAnnualVol   float64
	MinMultiplier     float64
	MaxMultiplier     float64
	MinObservations   int
}
type VolatilityTargetResult struct {
	Allowed          bool     `json:"allowed"`
	Observations     int      `json:"observations"`
	AnnualizedVol    float64  `json:"annualized_vol"`
	Multiplier       float64  `json:"multiplier"`
	AdjustedNotional float64  `json:"adjusted_notional"`
	Reasons          []string `json:"reasons,omitempty"`
}

// EvaluateVolatilityTarget scales notional inversely to annualized realized
// volatility. It only reduces risk in production when MaxMultiplier <= 1.
func EvaluateVolatilityTarget(in VolatilityTargetInput) VolatilityTargetResult {
	r := VolatilityTargetResult{Allowed: true, Multiplier: 1, AdjustedNotional: in.RequestedNotional}
	if in.MinObservations < 10 {
		in.MinObservations = 30
	}
	if in.TargetAnnualVol <= 0 {
		return r
	}
	returns := []float64{}
	for i := 1; i < len(in.Candles); i++ {
		a, b := in.Candles[i-1].Close, in.Candles[i].Close
		if a > 0 && b > 0 {
			returns = append(returns, math.Log(b/a))
		}
	}
	r.Observations = len(returns)
	if r.Observations < in.MinObservations {
		r.Allowed = false
		r.Reasons = append(r.Reasons, fmt.Sprintf("volatility history insufficient: %d/%d", r.Observations, in.MinObservations))
		return r
	}
	mean := 0.0
	for _, v := range returns {
		mean += v
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, v := range returns {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(returns) - 1)
	r.AnnualizedVol = math.Sqrt(variance) * math.Sqrt(365)
	if math.IsNaN(r.AnnualizedVol) || math.IsInf(r.AnnualizedVol, 0) || r.AnnualizedVol <= 0 {
		r.Allowed = false
		r.Reasons = append(r.Reasons, "realized volatility invalid")
		return r
	}
	r.Multiplier = in.TargetAnnualVol / r.AnnualizedVol
	if in.MinMultiplier > 0 && r.Multiplier < in.MinMultiplier {
		r.Multiplier = in.MinMultiplier
	}
	max := in.MaxMultiplier
	if max <= 0 || max > 1 {
		max = 1
	}
	if r.Multiplier > max {
		r.Multiplier = max
	}
	r.AdjustedNotional = in.RequestedNotional * r.Multiplier
	if r.AdjustedNotional <= 0 {
		r.Allowed = false
		r.Reasons = append(r.Reasons, "volatility-adjusted notional is zero")
	}
	return r
}
