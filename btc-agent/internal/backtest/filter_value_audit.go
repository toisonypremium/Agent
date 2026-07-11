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
	FilterValueKeepStrict     = "KEEP_STRICT"
	FilterValueTuneReview     = "TUNE_REVIEW"
	FilterValueNeedMoreData   = "NEED_MORE_DATA"
	FilterValueDangerousRelax = "DANGEROUS_TO_RELAX"
)

type FilterValueAuditConfig struct {
	MinWindow1D   int      `json:"min_window_1d"`
	HorizonDays   []int    `json:"horizon_days"`
	TargetSymbols []string `json:"target_symbols"`
}

type FilterValueAuditResult struct {
	Enabled bool                  `json:"enabled"`
	Rows    []FilterValueAuditRow `json:"rows"`
	Summary string                `json:"summary"`
}

type FilterValueAuditRow struct {
	Filter                  string          `json:"filter"`
	Samples                 int             `json:"samples"`
	Blocked                 int             `json:"blocked"`
	Passed                  int             `json:"passed"`
	BlockedAvgForwardReturn map[int]float64 `json:"blocked_avg_forward_return"`
	PassedAvgForwardReturn  map[int]float64 `json:"passed_avg_forward_return"`
	BlockedWinRate          map[int]float64 `json:"blocked_win_rate"`
	PassedWinRate           map[int]float64 `json:"passed_win_rate"`
	WorstDrawdown           map[int]float64 `json:"worst_drawdown"`
	FalseNegativeRate       float64         `json:"false_negative_rate"`
	Verdict                 string          `json:"verdict"`
}

type filterValueAcc struct {
	blocked       int
	passed        int
	blockedReturn map[int]float64
	passedReturn  map[int]float64
	blockedWins   map[int]int
	passedWins    map[int]int
	worstDD       map[int]float64
	initDD        map[int]bool
	falseNegative int
}

func RunFilterValueAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg FilterValueAuditConfig) (FilterValueAuditResult, error) {
	auditCfg = normalizeFilterValueAuditConfig(cfg, auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return FilterValueAuditResult{}, fmt.Errorf("not enough BTC 1d candles for filter value audit; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < need {
		return FilterValueAuditResult{}, fmt.Errorf("not enough aligned asset candles for filter value audit; need %d got %d", need, lastIndex+1)
	}
	accs := map[string]*filterValueAcc{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := auditCfg.MinWindow1D; i+maxH <= lastIndex; i++ {
		btcWindow := btcTimeframeWindow(btc, i)
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		assetWindows := map[string][]market.Candle{}
		for _, sym := range auditCfg.TargetSymbols {
			if len(assets[sym]) > i+maxH {
				assetWindows[sym] = assets[sym][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assetWindows, benchmarks)
		for _, asset := range plan.Assets {
			candles := assets[asset.Symbol]
			if len(candles) <= i+maxH || candles[i].Close <= 0 {
				continue
			}
			for _, gate := range asset.SetupGates {
				key := agent2.NormalizeReasonKey(gate.Name)
				acc := accs[key]
				if acc == nil {
					acc = newFilterValueAcc(auditCfg.HorizonDays)
					accs[key] = acc
				}
				accumulateFilterValue(acc, gate, asset, candles, i, auditCfg.HorizonDays)
			}
		}
	}
	result := FilterValueAuditResult{Enabled: true}
	for key, acc := range accs {
		result.Rows = append(result.Rows, finalizeFilterValueRow(key, acc, auditCfg.HorizonDays))
	}
	sortFilterValueRows(result.Rows)
	result.Summary = summarizeFilterValueAudit(result.Rows)
	return result, nil
}

func normalizeFilterValueAuditConfig(cfg config.Config, auditCfg FilterValueAuditConfig) FilterValueAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	seen := map[int]bool{}
	out := []int{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	sort.Ints(out)
	auditCfg.HorizonDays = out
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func newFilterValueAcc(horizons []int) *filterValueAcc {
	return &filterValueAcc{blockedReturn: map[int]float64{}, passedReturn: map[int]float64{}, blockedWins: map[int]int{}, passedWins: map[int]int{}, worstDD: map[int]float64{}, initDD: map[int]bool{}}
}

func accumulateFilterValue(acc *filterValueAcc, gate agent2.SetupGateResult, asset agent2.AssetPlan, candles []market.Candle, i int, horizons []int) {
	entry := candles[i].Close
	if entry <= 0 {
		return
	}
	if gate.Pass {
		acc.passed++
	} else {
		acc.blocked++
	}
	softOnly := gate.Severity != agent2.SetupGateHard && !hasHardSetupFailure(asset)
	falseCandidate := false
	for _, h := range horizons {
		ret := (candles[i+h].Close - entry) / entry
		dd := worstDrawdown(candles[i+1:i+h+1], entry)
		if gate.Pass {
			acc.passedReturn[h] += ret
			if ret > 0 {
				acc.passedWins[h]++
			}
		} else {
			acc.blockedReturn[h] += ret
			if ret > 0 {
				acc.blockedWins[h]++
			}
			if h == 7 && softOnly && ret >= 0.03 && dd > -0.08 {
				falseCandidate = true
			}
		}
		if !acc.initDD[h] || dd < acc.worstDD[h] {
			acc.worstDD[h] = dd
			acc.initDD[h] = true
		}
	}
	if falseCandidate {
		acc.falseNegative++
	}
}

func hasHardSetupFailure(asset agent2.AssetPlan) bool {
	for _, gate := range asset.SetupGates {
		if !gate.Pass && gate.Severity == agent2.SetupGateHard {
			return true
		}
	}
	return false
}

func finalizeFilterValueRow(key string, acc *filterValueAcc, horizons []int) FilterValueAuditRow {
	row := FilterValueAuditRow{Filter: key, BlockedAvgForwardReturn: map[int]float64{}, PassedAvgForwardReturn: map[int]float64{}, BlockedWinRate: map[int]float64{}, PassedWinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if acc == nil {
		row.Verdict = FilterValueNeedMoreData
		return row
	}
	row.Blocked = acc.blocked
	row.Passed = acc.passed
	row.Samples = acc.blocked + acc.passed
	for _, h := range horizons {
		if acc.blocked > 0 {
			row.BlockedAvgForwardReturn[h] = acc.blockedReturn[h] / float64(acc.blocked)
			row.BlockedWinRate[h] = float64(acc.blockedWins[h]) / float64(acc.blocked)
		}
		if acc.passed > 0 {
			row.PassedAvgForwardReturn[h] = acc.passedReturn[h] / float64(acc.passed)
			row.PassedWinRate[h] = float64(acc.passedWins[h]) / float64(acc.passed)
		}
		row.WorstDrawdown[h] = acc.worstDD[h]
	}
	if acc.blocked > 0 {
		row.FalseNegativeRate = float64(acc.falseNegative) / float64(acc.blocked)
	}
	row.Verdict = filterValueVerdict(row)
	return row
}

func filterValueVerdict(row FilterValueAuditRow) string {
	if row.Samples < 20 || row.Blocked < 5 {
		return FilterValueNeedMoreData
	}
	if row.WorstDrawdown[7] <= -0.12 || row.BlockedWinRate[7] < 0.40 {
		return FilterValueDangerousRelax
	}
	if row.FalseNegativeRate >= 0.35 && row.BlockedAvgForwardReturn[7] > 0 {
		return FilterValueTuneReview
	}
	return FilterValueKeepStrict
}

func sortFilterValueRows(rows []FilterValueAuditRow) {
	sort.Slice(rows, func(i, j int) bool {
		order := map[string]int{FilterValueTuneReview: 0, FilterValueDangerousRelax: 1, FilterValueKeepStrict: 2, FilterValueNeedMoreData: 3}
		if order[rows[i].Verdict] != order[rows[j].Verdict] {
			return order[rows[i].Verdict] < order[rows[j].Verdict]
		}
		if rows[i].FalseNegativeRate != rows[j].FalseNegativeRate {
			return rows[i].FalseNegativeRate > rows[j].FalseNegativeRate
		}
		return rows[i].Filter < rows[j].Filter
	})
}

func summarizeFilterValueAudit(rows []FilterValueAuditRow) string {
	if len(rows) == 0 {
		return "Filter value audit produced no rows."
	}
	tune := 0
	danger := 0
	for _, row := range rows {
		switch row.Verdict {
		case FilterValueTuneReview:
			tune++
		case FilterValueDangerousRelax:
			danger++
		}
	}
	return fmt.Sprintf("Filter value audit filters=%d tune_review=%d dangerous_to_relax=%d; research-only, no production rule changed", len(rows), tune, danger)
}
