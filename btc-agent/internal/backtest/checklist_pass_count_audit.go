package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
)

const (
	ChecklistVerdictDataWait            = "DATA_WAIT"
	ChecklistVerdictBlocked             = "BLOCKED"
	ChecklistVerdictWatch               = "WATCH"
	ChecklistVerdictNearActionableWatch = "NEAR_ACTIONABLE_WATCH"
)

type ChecklistPassCountAuditConfig struct {
	MinWindow1D           int      `json:"min_window_1d"`
	TargetSymbols         []string `json:"target_symbols"`
	NearActionableSoftMax int      `json:"near_actionable_soft_max"`
}

type ChecklistPassCountAuditResult struct {
	Enabled bool                         `json:"enabled"`
	Rows    []ChecklistPassCountAuditRow `json:"rows"`
	Summary string                       `json:"summary"`
}

type ChecklistPassCountAuditRow struct {
	Symbol              string         `json:"symbol"`
	Samples             int            `json:"samples"`
	AvgPassedChecks     float64        `json:"avg_passed_checks"`
	AvgTotalChecks      float64        `json:"avg_total_checks"`
	HardFailRate        float64        `json:"hard_fail_rate"`
	SoftFailRate        float64        `json:"soft_fail_rate"`
	NearActionableCount int            `json:"near_actionable_count"`
	TopHardBlocker      string         `json:"top_hard_blocker"`
	TopSoftWait         string         `json:"top_soft_wait"`
	CheckFailCounts     map[string]int `json:"check_fail_counts"`
	HardFailCounts      map[string]int `json:"hard_fail_counts"`
	SoftFailCounts      map[string]int `json:"soft_fail_counts"`
	Verdict             string         `json:"verdict"`
}

type checklistAuditAcc struct {
	samples             int
	passedTotal         int
	totalChecks         int
	hardFailSamples     int
	softFailSamples     int
	nearActionableCount int
	checkFailCounts     map[string]int
	hardFailCounts      map[string]int
	softFailCounts      map[string]int
}

func RunChecklistPassCountAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg ChecklistPassCountAuditConfig) (ChecklistPassCountAuditResult, error) {
	auditCfg = normalizeChecklistPassCountAuditConfig(cfg, auditCfg)
	btc1d := btc["1d"]
	need := auditCfg.MinWindow1D + 1
	if len(btc1d) < need {
		return ChecklistPassCountAuditResult{}, fmt.Errorf("not enough BTC 1d candles for checklist pass-count audit; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < auditCfg.MinWindow1D {
		return ChecklistPassCountAuditResult{}, fmt.Errorf("not enough aligned asset candles for checklist pass-count audit; need %d got %d", need, lastIndex+1)
	}

	acc := map[string]*checklistAuditAcc{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := auditCfg.MinWindow1D; i <= lastIndex; i++ {
		btcWindow := map[string][]market.Candle{"1d": btc1d[:i+1], "4h": btc1d[:i+1], "1w": btc1d[:i+1]}
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		assetWindows := map[string][]market.Candle{}
		for _, sym := range auditCfg.TargetSymbols {
			if len(assets[sym]) > i {
				assetWindows[sym] = assets[sym][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assetWindows, benchmarks)
		for _, candidate := range plan.Watchlist.Candidates {
			if !containsSymbol(auditCfg.TargetSymbols, candidate.Symbol) {
				continue
			}
			a := acc[candidate.Symbol]
			if a == nil {
				a = newChecklistAuditAcc()
				acc[candidate.Symbol] = a
			}
			accumulateChecklistCandidate(a, candidate, auditCfg.NearActionableSoftMax)
		}
	}

	result := ChecklistPassCountAuditResult{Enabled: true}
	for sym, a := range acc {
		result.Rows = append(result.Rows, finalizeChecklistAuditRow(sym, a))
	}
	sortChecklistPassCountRows(result.Rows)
	result.Summary = summarizeChecklistPassCountAudit(result.Rows)
	return result, nil
}

func normalizeChecklistPassCountAuditConfig(cfg config.Config, auditCfg ChecklistPassCountAuditConfig) ChecklistPassCountAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	if auditCfg.NearActionableSoftMax <= 0 {
		auditCfg.NearActionableSoftMax = 2
	}
	return auditCfg
}

func newChecklistAuditAcc() *checklistAuditAcc {
	return &checklistAuditAcc{checkFailCounts: map[string]int{}, hardFailCounts: map[string]int{}, softFailCounts: map[string]int{}}
}

func accumulateChecklistCandidate(a *checklistAuditAcc, candidate agent2.WatchCandidate, nearActionableSoftMax int) {
	a.samples++
	hardFails := 0
	softFails := 0
	for _, item := range candidate.EntryChecklist {
		a.totalChecks++
		if item.Pass {
			a.passedTotal++
			continue
		}
		a.checkFailCounts[item.Name]++
		if item.Severity == agent2.EntryCheckHard {
			hardFails++
			a.hardFailCounts[item.Name]++
		} else {
			softFails++
			a.softFailCounts[item.Name]++
		}
	}
	if hardFails > 0 {
		a.hardFailSamples++
	}
	if softFails > 0 {
		a.softFailSamples++
	}
	if len(candidate.EntryChecklist) > 0 && hardFails == 0 && softFails <= nearActionableSoftMax {
		a.nearActionableCount++
	}
}

func finalizeChecklistAuditRow(symbol string, a *checklistAuditAcc) ChecklistPassCountAuditRow {
	row := ChecklistPassCountAuditRow{Symbol: symbol, CheckFailCounts: copyIntMap(a.checkFailCounts), HardFailCounts: copyIntMap(a.hardFailCounts), SoftFailCounts: copyIntMap(a.softFailCounts)}
	row.Samples = a.samples
	if a.samples > 0 {
		row.AvgPassedChecks = float64(a.passedTotal) / float64(a.samples)
		row.AvgTotalChecks = float64(a.totalChecks) / float64(a.samples)
		row.HardFailRate = float64(a.hardFailSamples) / float64(a.samples)
		row.SoftFailRate = float64(a.softFailSamples) / float64(a.samples)
	}
	row.NearActionableCount = a.nearActionableCount
	row.TopHardBlocker = topCountKey(a.hardFailCounts)
	row.TopSoftWait = topCountKey(a.softFailCounts)
	row.Verdict = checklistPassCountVerdict(row)
	return row
}

func checklistPassCountVerdict(row ChecklistPassCountAuditRow) string {
	if row.Samples == 0 {
		return ChecklistVerdictDataWait
	}
	if row.HardFailRate >= 0.70 {
		return ChecklistVerdictBlocked
	}
	if row.NearActionableCount >= 5 && row.HardFailRate < 0.50 {
		return ChecklistVerdictNearActionableWatch
	}
	return ChecklistVerdictWatch
}

func sortChecklistPassCountRows(rows []ChecklistPassCountAuditRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].NearActionableCount != rows[j].NearActionableCount {
			return rows[i].NearActionableCount > rows[j].NearActionableCount
		}
		if rows[i].HardFailRate != rows[j].HardFailRate {
			return rows[i].HardFailRate < rows[j].HardFailRate
		}
		if rows[i].AvgPassedChecks != rows[j].AvgPassedChecks {
			return rows[i].AvgPassedChecks > rows[j].AvgPassedChecks
		}
		return rows[i].Symbol < rows[j].Symbol
	})
}

func summarizeChecklistPassCountAudit(rows []ChecklistPassCountAuditRow) string {
	if len(rows) == 0 {
		return "Checklist pass-count audit produced no rows; not enough aligned watchlist samples."
	}
	samples := 0
	near := 0
	for _, row := range rows {
		samples += row.Samples
		near += row.NearActionableCount
	}
	best := rows[0]
	topBlocker := best.TopHardBlocker
	if topBlocker == "" {
		topBlocker = best.TopSoftWait
	}
	return fmt.Sprintf("Checklist audit rows=%d samples=%d near_actionable=%d top_near=%s top_blocker=%s", len(rows), samples, near, best.Symbol, topBlocker)
}

func topCountKey(counts map[string]int) string {
	bestKey := ""
	bestVal := 0
	for key, val := range counts {
		if val > bestVal || (val == bestVal && (bestKey == "" || key < bestKey)) {
			bestKey = key
			bestVal = val
		}
	}
	return bestKey
}

func copyIntMap(in map[string]int) map[string]int {
	out := map[string]int{}
	for key, val := range in {
		out[key] = val
	}
	return out
}

func containsSymbol(symbols []string, want string) bool {
	for _, sym := range symbols {
		if sym == want {
			return true
		}
	}
	return false
}
