package liveguard

import (
	"btc-agent/internal/market"
	"fmt"
	"math"
	"sort"
	"strings"
)

type CorrelationBudgetInput struct {
	Symbol            string
	RequestedNotional float64
	ExistingExposure  map[string]float64
	Candles           map[string][]market.Candle
	Threshold         float64
	ClusterCap        float64
	MinObservations   int
}
type CorrelationBudgetResult struct {
	Allowed             bool               `json:"allowed"`
	CorrelationExposure float64            `json:"correlation_exposure"`
	Remaining           float64            `json:"remaining"`
	AdjustedNotional    float64            `json:"adjusted_notional"`
	Correlated          map[string]float64 `json:"correlated,omitempty"`
	Reasons             []string           `json:"reasons,omitempty"`
}

func EvaluateCorrelationBudget(in CorrelationBudgetInput) CorrelationBudgetResult {
	r := CorrelationBudgetResult{Allowed: true, AdjustedNotional: in.RequestedNotional, Correlated: map[string]float64{}}
	symbol := strings.ToUpper(in.Symbol)
	if in.ClusterCap <= 0 || in.Threshold <= 0 {
		return r
	}
	if in.MinObservations < 10 {
		in.MinObservations = 30
	}
	r.CorrelationExposure = in.ExistingExposure[symbol]
	for other, exposure := range in.ExistingExposure {
		other = strings.ToUpper(other)
		if other == symbol || exposure <= 0 {
			continue
		}
		corr, n := AlignedReturnCorrelation(in.Candles[symbol], in.Candles[other])
		if n < in.MinObservations {
			r.Allowed = false
			r.Reasons = append(r.Reasons, fmt.Sprintf("correlation history insufficient for %s/%s: %d", symbol, other, n))
			continue
		}
		if corr >= in.Threshold {
			r.Correlated[other] = corr
			r.CorrelationExposure += exposure
		}
	}
	r.Remaining = in.ClusterCap - r.CorrelationExposure
	if r.Remaining < 0 {
		r.Remaining = 0
	}
	if r.AdjustedNotional > r.Remaining {
		r.AdjustedNotional = r.Remaining
	}
	if r.AdjustedNotional <= 0 {
		r.Allowed = false
		r.Reasons = append(r.Reasons, "correlated exposure cap exhausted")
	}
	return r
}
func AlignedReturnCorrelation(a, b []market.Candle) (float64, int) {
	ma := returnsByTime(a)
	mb := returnsByTime(b)
	keys := []int64{}
	for k := range ma {
		if _, ok := mb[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	n := len(keys)
	if n < 2 {
		return 0, n
	}
	xa, xb := make([]float64, n), make([]float64, n)
	for i, k := range keys {
		xa[i], xb[i] = ma[k], mb[k]
	}
	av, bv := meanCorrelation(xa), meanCorrelation(xb)
	num, da, db := 0.0, 0.0, 0.0
	for i := range xa {
		x, y := xa[i]-av, xb[i]-bv
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0, n
	}
	return num / math.Sqrt(da*db), n
}
func returnsByTime(cs []market.Candle) map[int64]float64 {
	out := map[int64]float64{}
	for i := 1; i < len(cs); i++ {
		if cs[i-1].Close > 0 && cs[i].Close > 0 {
			out[cs[i].OpenTime.Unix()] = cs[i].Close/cs[i-1].Close - 1
		}
	}
	return out
}
func meanCorrelation(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}
