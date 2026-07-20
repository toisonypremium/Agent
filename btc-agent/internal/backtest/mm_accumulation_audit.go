package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

type MMAccumulationAuditResult struct {
	Enabled        bool                      `json:"enabled"`
	EvidenceSource string                    `json:"evidence_source"`
	Horizons       []int                     `json:"horizons"`
	Rows           []MMAccumulationAuditRow  `json:"rows"`
	TopMissing     []MMAccumulationAuditItem `json:"top_missing,omitempty"`
	Summary        string                    `json:"summary"`
}

type MMAccumulationAuditRow struct {
	Symbol            string                               `json:"symbol"`
	Samples           int                                  `json:"samples"`
	CaseCounts        map[agent2.MMCase]int                `json:"case_counts"`
	AvgScore          float64                              `json:"avg_score"`
	PassRate          float64                              `json:"pass_rate"`
	HardBlockRate     float64                              `json:"hard_block_rate"`
	LiquidityPassRate float64                              `json:"liquidity_pass_rate"`
	AvgLiquidityScore float64                              `json:"avg_liquidity_score"`
	TopCase           agent2.MMCase                        `json:"top_case"`
	ByCase            map[agent2.MMCase]MMCaseForwardStats `json:"by_case"`
	TopMissing        []MMAccumulationAuditItem            `json:"top_missing,omitempty"`
}

type MMAccumulationAuditItem struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// MMCaseForwardStats evaluates predictive value without changing any threshold or authority.
type MMCaseForwardStats struct {
	Samples       int             `json:"samples"`
	AvgReturn     map[int]float64 `json:"avg_return"`
	WinRate       map[int]float64 `json:"win_rate"`
	AvgMAE        map[int]float64 `json:"avg_mae"`
	AvgMFE        map[int]float64 `json:"avg_mfe"`
	WorstMAE      map[int]float64 `json:"worst_mae"`
	BestMFE       map[int]float64 `json:"best_mfe"`
	SampleQuality string          `json:"sample_quality"`
}

type mmForwardAcc struct {
	count             int
	returns, mae, mfe map[int]float64
	wins              map[int]int
	worstMAE, bestMFE map[int]float64
}

func RunMMAccumulationAudit(cfg config.Config, assets map[string][]market.Candle) MMAccumulationAuditResult {
	horizons := []int{1, 3, 7, 14, 30}
	result := MMAccumulationAuditResult{Enabled: true, EvidenceSource: "OHLCV_ACCUMULATION_STRUCTURE", Horizons: horizons}
	globalMissing := map[string]int{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		candles := assets[symbol]
		if len(candles) < 80 {
			continue
		}
		row := MMAccumulationAuditRow{Symbol: symbol, CaseCounts: map[agent2.MMCase]int{}, ByCase: map[agent2.MMCase]MMCaseForwardStats{}}
		forward := map[agent2.MMCase]*mmForwardAcc{}
		missing := map[string]int{}
		maxH := 30
		for i := 60; i+maxH < len(candles); i++ {
			window := candles[:i]
			mm := agent2.AnalyzeMMAccumulation(symbol, window)
			q := liquidity.EvaluateCandleProxy(cfg, symbol, window, 1)
			row.Samples++
			row.CaseCounts[mm.Case]++
			acc := forward[mm.Case]
			if acc == nil {
				acc = newMMForwardAcc(horizons)
				forward[mm.Case] = acc
			}
			acc.add(candles, i-1, horizons)
			row.AvgScore += mm.Score
			row.AvgLiquidityScore += q.Score
			if mm.Pass {
				row.PassRate++
			}
			if mm.HardBlock {
				row.HardBlockRate++
			}
			if q.Pass {
				row.LiquidityPassRate++
			}
			for _, m := range mm.Missing {
				missing[m]++
				globalMissing[symbol+": "+m]++
			}
			if !q.Pass && len(q.Reasons) > 0 {
				missing[q.Reasons[0]]++
				globalMissing[symbol+": "+q.Reasons[0]]++
			}
		}
		if row.Samples == 0 {
			continue
		}
		row.AvgScore /= float64(row.Samples)
		row.AvgLiquidityScore /= float64(row.Samples)
		row.PassRate /= float64(row.Samples)
		row.HardBlockRate /= float64(row.Samples)
		row.LiquidityPassRate /= float64(row.Samples)
		row.TopCase = topMMCase(row.CaseCounts)
		for c, acc := range forward {
			row.ByCase[c] = acc.finalize(horizons)
		}
		row.TopMissing = topAuditItems(missing, 3)
		result.Rows = append(result.Rows, row)
	}
	result.TopMissing = topAuditItems(globalMissing, 8)
	if len(result.Rows) == 0 {
		result.Enabled = false
		result.Summary = "OHLCV accumulation structure audit skipped: not enough asset candles"
		return result
	}
	result.Summary = fmt.Sprintf("OHLCV accumulation structure audit: symbols=%d samples=%d horizons=1/3/7/14/30D", len(result.Rows), totalMMSamples(result.Rows))
	return result
}

func topMMCase(counts map[agent2.MMCase]int) agent2.MMCase {
	best := agent2.MMCaseNoEdge
	bestCount := -1
	for c, n := range counts {
		if n > bestCount || n == bestCount && string(c) < string(best) {
			best = c
			bestCount = n
		}
	}
	return best
}

func topAuditItems(counts map[string]int, limit int) []MMAccumulationAuditItem {
	items := make([]MMAccumulationAuditItem, 0, len(counts))
	for reason, count := range counts {
		if reason == "" || count <= 0 {
			continue
		}
		items = append(items, MMAccumulationAuditItem{Reason: reason, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Reason < items[j].Reason
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func totalMMSamples(rows []MMAccumulationAuditRow) int {
	total := 0
	for _, row := range rows {
		total += row.Samples
	}
	return total
}

func newMMForwardAcc(horizons []int) *mmForwardAcc {
	a := &mmForwardAcc{returns: map[int]float64{}, mae: map[int]float64{}, mfe: map[int]float64{}, wins: map[int]int{}, worstMAE: map[int]float64{}, bestMFE: map[int]float64{}}
	for _, h := range horizons {
		a.worstMAE[h] = 1
		a.bestMFE[h] = -1
	}
	return a
}

func (a *mmForwardAcc) add(c []market.Candle, entryIndex int, horizons []int) {
	entry := c[entryIndex].Close
	if entry <= 0 {
		return
	}
	a.count++
	for _, h := range horizons {
		future := c[entryIndex+h]
		ret := (future.Close - entry) / entry
		mae, mfe := 0.0, 0.0
		for _, x := range c[entryIndex+1 : entryIndex+h+1] {
			low, high := (x.Low-entry)/entry, (x.High-entry)/entry
			if low < mae {
				mae = low
			}
			if high > mfe {
				mfe = high
			}
		}
		a.returns[h] += ret
		a.mae[h] += mae
		a.mfe[h] += mfe
		if ret > 0 {
			a.wins[h]++
		}
		if mae < a.worstMAE[h] {
			a.worstMAE[h] = mae
		}
		if mfe > a.bestMFE[h] {
			a.bestMFE[h] = mfe
		}
	}
}

func (a *mmForwardAcc) finalize(horizons []int) MMCaseForwardStats {
	r := MMCaseForwardStats{Samples: a.count, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, AvgMAE: map[int]float64{}, AvgMFE: map[int]float64{}, WorstMAE: map[int]float64{}, BestMFE: map[int]float64{}, SampleQuality: "INSUFFICIENT_SAMPLE"}
	if a.count >= 30 {
		r.SampleQuality = "ADEQUATE"
	} else if a.count >= 10 {
		r.SampleQuality = "LIMITED"
	}
	if a.count == 0 {
		return r
	}
	for _, h := range horizons {
		n := float64(a.count)
		r.AvgReturn[h] = a.returns[h] / n
		r.WinRate[h] = float64(a.wins[h]) / n
		r.AvgMAE[h] = a.mae[h] / n
		r.AvgMFE[h] = a.mfe[h] / n
		r.WorstMAE[h] = a.worstMAE[h]
		r.BestMFE[h] = a.bestMFE[h]
	}
	return r
}
