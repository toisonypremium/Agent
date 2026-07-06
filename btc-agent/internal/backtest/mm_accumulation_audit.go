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
	Enabled    bool                      `json:"enabled"`
	Rows       []MMAccumulationAuditRow  `json:"rows"`
	TopMissing []MMAccumulationAuditItem `json:"top_missing,omitempty"`
	Summary    string                    `json:"summary"`
}

type MMAccumulationAuditRow struct {
	Symbol            string                    `json:"symbol"`
	Samples           int                       `json:"samples"`
	CaseCounts        map[agent2.MMCase]int     `json:"case_counts"`
	AvgScore          float64                   `json:"avg_score"`
	PassRate          float64                   `json:"pass_rate"`
	HardBlockRate     float64                   `json:"hard_block_rate"`
	LiquidityPassRate float64                   `json:"liquidity_pass_rate"`
	AvgLiquidityScore float64                   `json:"avg_liquidity_score"`
	TopCase           agent2.MMCase             `json:"top_case"`
	TopMissing        []MMAccumulationAuditItem `json:"top_missing,omitempty"`
}

type MMAccumulationAuditItem struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func RunMMAccumulationAudit(cfg config.Config, assets map[string][]market.Candle) MMAccumulationAuditResult {
	result := MMAccumulationAuditResult{Enabled: true}
	globalMissing := map[string]int{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		candles := assets[symbol]
		if len(candles) < 80 {
			continue
		}
		row := MMAccumulationAuditRow{Symbol: symbol, CaseCounts: map[agent2.MMCase]int{}}
		missing := map[string]int{}
		for i := 60; i <= len(candles); i++ {
			window := candles[:i]
			mm := agent2.AnalyzeMMAccumulation(symbol, window)
			q := liquidity.EvaluateCandleProxy(cfg, symbol, window, 1)
			row.Samples++
			row.CaseCounts[mm.Case]++
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
		row.TopMissing = topAuditItems(missing, 3)
		result.Rows = append(result.Rows, row)
	}
	result.TopMissing = topAuditItems(globalMissing, 8)
	if len(result.Rows) == 0 {
		result.Enabled = false
		result.Summary = "MM accumulation audit skipped: not enough asset candles"
		return result
	}
	result.Summary = fmt.Sprintf("MM accumulation audit: symbols=%d samples=%d", len(result.Rows), totalMMSamples(result.Rows))
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
