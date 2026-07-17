package backtest

import (
	"fmt"
	"math/rand"
	"sort"
)

type MonteCarloReport struct {
	Enabled           bool    `json:"enabled"`
	Iterations        int     `json:"iterations"`
	Samples           int     `json:"samples"`
	Seed              int64   `json:"seed"`
	ProbabilityOfLoss float64 `json:"probability_of_loss"`
	P05FinalPnL       float64 `json:"p05_final_pnl"`
	MedianFinalPnL    float64 `json:"median_final_pnl"`
	P95FinalPnL       float64 `json:"p95_final_pnl"`
	P50MaxDrawdown    float64 `json:"p50_max_drawdown"`
	P95MaxDrawdown    float64 `json:"p95_max_drawdown"`
	Verdict           string  `json:"verdict"`
	Summary           string  `json:"summary"`
}

// RunMonteCarloRobustness bootstraps out-of-sample walk-forward asset results.
// It is deterministic for a fixed seed and research-only. Bootstrap estimates
// outcome uncertainty; shuffled order estimates path-dependent drawdown.
func RunMonteCarloRobustness(w WalkForwardReport, iterations int, seed int64) (MonteCarloReport, error) {
	if !w.Enabled {
		return MonteCarloReport{}, fmt.Errorf("walk-forward report unavailable")
	}
	if iterations < 100 {
		return MonteCarloReport{}, fmt.Errorf("iterations must be at least 100")
	}
	samples := []float64{}
	for _, split := range w.Splits {
		if !split.Eval.Enabled {
			continue
		}
		for _, a := range split.Eval.Assets {
			if a.OrdersFilled > 0 {
				samples = append(samples, a.FinalPnL)
			}
		}
	}
	if len(samples) < 3 {
		return MonteCarloReport{}, fmt.Errorf("not enough out-of-sample PnL observations; need 3 got %d", len(samples))
	}
	rng := rand.New(rand.NewSource(seed))
	finals := make([]float64, iterations)
	dds := make([]float64, iterations)
	losses := 0
	for i := 0; i < iterations; i++ {
		path := make([]float64, len(samples))
		for j := range path {
			path[j] = samples[rng.Intn(len(samples))]
		}
		rng.Shuffle(len(path), func(a, b int) { path[a], path[b] = path[b], path[a] })
		equity, peak, maxDD := 0.0, 0.0, 0.0
		for _, p := range path {
			equity += p
			if equity > peak {
				peak = equity
			}
			if d := peak - equity; d > maxDD {
				maxDD = d
			}
		}
		finals[i] = equity
		dds[i] = maxDD
		if equity < 0 {
			losses++
		}
	}
	sort.Float64s(finals)
	sort.Float64s(dds)
	q := func(x []float64, p float64) float64 { idx := int(p * float64(len(x)-1)); return x[idx] }
	r := MonteCarloReport{Enabled: true, Iterations: iterations, Samples: len(samples), Seed: seed, ProbabilityOfLoss: float64(losses) / float64(iterations), P05FinalPnL: q(finals, .05), MedianFinalPnL: q(finals, .50), P95FinalPnL: q(finals, .95), P50MaxDrawdown: q(dds, .50), P95MaxDrawdown: q(dds, .95)}
	switch {
	case r.ProbabilityOfLoss <= .20 && r.P05FinalPnL >= 0:
		r.Verdict = "ROBUST"
	case r.ProbabilityOfLoss <= .45:
		r.Verdict = "WATCH"
	default:
		r.Verdict = "FRAGILE"
	}
	r.Summary = fmt.Sprintf("Monte Carlo research-only: iterations=%d samples=%d loss_probability=%.1f%% p05_pnl=%.2f p95_drawdown=%.2f verdict=%s", r.Iterations, r.Samples, r.ProbabilityOfLoss*100, r.P05FinalPnL, r.P95MaxDrawdown, r.Verdict)
	return r, nil
}
