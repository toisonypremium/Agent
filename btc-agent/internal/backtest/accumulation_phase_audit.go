package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/market"
)

type AccumulationPhaseAuditResult struct {
	Enabled bool                        `json:"enabled"`
	Symbol  string                      `json:"symbol"`
	Rows    []AccumulationPhaseAuditRow `json:"rows"`
	Summary string                      `json:"summary"`
}

type AccumulationPhaseAuditRow struct {
	Phase             accumulation.Phase `json:"phase"`
	Count             int                `json:"count"`
	AvgScore          float64            `json:"avg_score"`
	AvgForwardReturn  map[int]float64    `json:"avg_forward_return"`
	WinRate           map[int]float64    `json:"win_rate"`
	WorstMAE          map[int]float64    `json:"worst_mae"`
	BestMFE           map[int]float64    `json:"best_mfe"`
	FalsePositiveRate float64            `json:"false_positive_rate"`
	Verdict           string             `json:"verdict"`
}

type accumulationPhaseAcc struct {
	count          int
	score          float64
	returns        map[int]float64
	wins           map[int]int
	worstMAE       map[int]float64
	bestMFE        map[int]float64
	initializedMAE map[int]bool
	initializedMFE map[int]bool
	falsePositive  int
}

func RunAccumulationPhaseAudit(symbol string, daily []market.Candle, horizons []int) AccumulationPhaseAuditResult {
	cfg := normalizeConfig(Config{MinWindow1D: 60, HorizonDays: horizons})
	maxH := maxHorizon(cfg.HorizonDays)
	need := cfg.MinWindow1D + maxH + 1
	if len(daily) < need {
		return AccumulationPhaseAuditResult{Enabled: false, Symbol: symbol, Summary: fmt.Sprintf("accumulation phase audit skipped: need %d candles, got %d", need, len(daily))}
	}
	return runAccumulationPhaseAuditRange(symbol, daily, cfg, cfg.MinWindow1D, len(daily)-maxH)
}

func runAccumulationPhaseAuditRange(symbol string, daily []market.Candle, cfg Config, start, end int) AccumulationPhaseAuditResult {
	maxH := maxHorizon(cfg.HorizonDays)
	if start < cfg.MinWindow1D {
		start = cfg.MinWindow1D
	}
	if limit := len(daily) - maxH; end > limit {
		end = limit
	}
	if start >= end {
		return AccumulationPhaseAuditResult{Enabled: false, Symbol: symbol, Summary: "accumulation phase audit range has no evaluable candles"}
	}
	result := AccumulationPhaseAuditResult{Enabled: true, Symbol: symbol}
	accs := map[accumulation.Phase]*accumulationPhaseAcc{}
	for _, phase := range allAccumulationPhases() {
		accs[phase] = newAccumulationPhaseAcc(cfg.HorizonDays)
	}
	for i := start; i < end; i++ {
		window := daily[:i+1]
		sig := accumulation.Analyze(symbol, window)
		bucket := accs[sig.Phase]
		if bucket == nil {
			bucket = newAccumulationPhaseAcc(cfg.HorizonDays)
			accs[sig.Phase] = bucket
		}
		entry := daily[i].Close
		if entry <= 0 {
			continue
		}
		bucket.count++
		bucket.score += sig.Score
		invalidation := sig.Support.Low * 0.985
		invalidated := false
		for _, h := range cfg.HorizonDays {
			future := daily[i+h]
			ret := (future.Close - entry) / entry
			mae := worstDrawdown(daily[i+1:i+h+1], entry)
			mfe := bestRunup(daily[i+1:i+h+1], entry)
			bucket.returns[h] += ret
			if ret > 0 {
				bucket.wins[h]++
			}
			if !bucket.initializedMAE[h] || mae < bucket.worstMAE[h] {
				bucket.worstMAE[h] = mae
				bucket.initializedMAE[h] = true
			}
			if !bucket.initializedMFE[h] || mfe > bucket.bestMFE[h] {
				bucket.bestMFE[h] = mfe
				bucket.initializedMFE[h] = true
			}
			if sig.Phase == accumulation.PhaseConfirmed && invalidation > 0 && touchedBelow(daily[i+1:i+h+1], invalidation) {
				invalidated = true
			}
		}
		if invalidated {
			bucket.falsePositive++
		}
	}
	for _, phase := range allAccumulationPhases() {
		row := finalizeAccumulationPhaseRow(phase, accs[phase], cfg.HorizonDays)
		if row.Count > 0 {
			result.Rows = append(result.Rows, row)
		}
	}
	result.Summary = fmt.Sprintf("accumulation phase audit: symbol=%s phases=%d samples=%d", symbol, len(result.Rows), totalAccumulationSamples(result.Rows))
	return result
}

func newAccumulationPhaseAcc(horizons []int) *accumulationPhaseAcc {
	a := &accumulationPhaseAcc{returns: map[int]float64{}, wins: map[int]int{}, worstMAE: map[int]float64{}, bestMFE: map[int]float64{}, initializedMAE: map[int]bool{}, initializedMFE: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstMAE[h] = 0
		a.bestMFE[h] = 0
	}
	return a
}

func finalizeAccumulationPhaseRow(phase accumulation.Phase, acc *accumulationPhaseAcc, horizons []int) AccumulationPhaseAuditRow {
	row := AccumulationPhaseAuditRow{Phase: phase, AvgForwardReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstMAE: map[int]float64{}, BestMFE: map[int]float64{}, Verdict: "NO_SAMPLE"}
	if acc == nil || acc.count == 0 {
		return row
	}
	row.Count = acc.count
	row.AvgScore = acc.score / float64(acc.count)
	for _, h := range horizons {
		row.AvgForwardReturn[h] = acc.returns[h] / float64(acc.count)
		row.WinRate[h] = float64(acc.wins[h]) / float64(acc.count)
		row.WorstMAE[h] = acc.worstMAE[h]
		row.BestMFE[h] = acc.bestMFE[h]
	}
	row.FalsePositiveRate = float64(acc.falsePositive) / float64(acc.count)
	row.Verdict = accumulationPhaseVerdict(row, horizons)
	return row
}

func accumulationPhaseVerdict(row AccumulationPhaseAuditRow, horizons []int) string {
	if row.Count < 10 {
		return "LOW_SAMPLE"
	}
	lastH := horizons[len(horizons)-1]
	if row.Phase == accumulation.PhaseConfirmed {
		if row.FalsePositiveRate > 0.35 || row.WorstMAE[lastH] < -0.12 {
			return "FALSE_POSITIVE_RISK"
		}
		if row.AvgForwardReturn[lastH] > 0 && row.WinRate[lastH] >= 0.50 {
			return "CANDIDATE"
		}
	}
	return "WATCH"
}

func bestRunup(c []market.Candle, entry float64) float64 {
	best := 0.0
	for _, candle := range c {
		runup := (candle.High - entry) / entry
		if runup > best {
			best = runup
		}
	}
	return best
}

func touchedBelow(c []market.Candle, price float64) bool {
	for _, candle := range c {
		if candle.Low <= price {
			return true
		}
	}
	return false
}

func allAccumulationPhases() []accumulation.Phase {
	return []accumulation.Phase{accumulation.PhaseMarkdown, accumulation.PhaseSweep, accumulation.PhaseAbsorption, accumulation.PhaseReclaim, accumulation.PhaseConfirmed, accumulation.PhaseDistribution, accumulation.PhaseInvalidated}
}

func totalAccumulationSamples(rows []AccumulationPhaseAuditRow) int {
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return total
}

func SortedAccumulationPhaseRows(rows []AccumulationPhaseAuditRow) []AccumulationPhaseAuditRow {
	out := append([]AccumulationPhaseAuditRow(nil), rows...)
	order := map[accumulation.Phase]int{}
	for i, phase := range allAccumulationPhases() {
		order[phase] = i
	}
	sort.Slice(out, func(i, j int) bool {
		return order[out[i].Phase] < order[out[j].Phase]
	})
	return out
}
