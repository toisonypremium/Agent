package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	BTCFlowRegimeVerdictCandidate = "CANDIDATE"
	BTCFlowRegimeVerdictWatch     = "WATCH"
	BTCFlowRegimeVerdictReject    = "REJECT"
	BTCFlowRegimeVerdictLowSample = "LOW_SAMPLE"
)

type BTCFlowRegimeAuditConfig struct {
	MinWindow1D int   `json:"min_window_1d"`
	HorizonDays []int `json:"horizon_days"`
}

type BTCFlowRegimeAuditResult struct {
	Enabled bool                    `json:"enabled"`
	Rows    []BTCFlowRegimeAuditRow `json:"rows"`
	Summary string                  `json:"summary"`
}

type BTCFlowRegimeAuditRow struct {
	Regime        string          `json:"regime"`
	Bias          flow.Bias       `json:"bias"`
	Count         int             `json:"count"`
	Rate          float64         `json:"rate"`
	AvgTrendScore float64         `json:"avg_trend_score"`
	AvgFlowScore  float64         `json:"avg_flow_score"`
	AvgReturn     map[int]float64 `json:"avg_return"`
	WinRate       map[int]float64 `json:"win_rate"`
	WorstDrawdown map[int]float64 `json:"worst_drawdown"`
	Verdict       string          `json:"verdict"`
}

type btcFlowRegimeKey struct {
	regime string
	bias   flow.Bias
}

type btcFlowRegimeAcc struct {
	count       int
	trendTotal  float64
	flowTotal   float64
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
}

func RunBTCFlowRegimeAudit(cfg config.Config, btc map[string][]market.Candle, auditCfg BTCFlowRegimeAuditConfig) (BTCFlowRegimeAuditResult, error) {
	auditCfg = normalizeBTCFlowRegimeAuditConfig(auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return BTCFlowRegimeAuditResult{}, fmt.Errorf("not enough BTC 1d candles for BTC flow regime audit; need %d got %d", need, len(btc1d))
	}

	acc := map[btcFlowRegimeKey]*btcFlowRegimeAcc{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	windows := 0
	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		btcWindow := btcTimeframeWindow(btc, i)
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		entry := btc1d[i].Close
		if entry <= 0 {
			continue
		}
		windows++
		bias := analysis.Flow.Bias
		if bias == "" {
			bias = flow.BiasNeutral
		}
		key := btcFlowRegimeKey{regime: analysis.MarketRegime, bias: bias}
		a := acc[key]
		if a == nil {
			a = newBTCFlowRegimeAcc(auditCfg.HorizonDays)
			acc[key] = a
		}
		a.count++
		a.trendTotal += analysis.TrendScore
		a.flowTotal += analysis.Flow.Score
		for _, h := range auditCfg.HorizonDays {
			future := btc1d[i+h]
			ret := (future.Close - entry) / entry
			dd := worstDrawdown(btc1d[i+1:i+h+1], entry)
			a.returns[h] += ret
			if ret > 0 {
				a.wins[h]++
			}
			if !a.initialized[h] || dd < a.worstDD[h] {
				a.worstDD[h] = dd
				a.initialized[h] = true
			}
		}
	}

	result := BTCFlowRegimeAuditResult{Enabled: true}
	for key, a := range acc {
		row := finalizeBTCFlowRegimeRow(key, a, auditCfg.HorizonDays, windows)
		row.Verdict = btcFlowRegimeVerdict(row, auditCfg.HorizonDays)
		result.Rows = append(result.Rows, row)
	}
	sortBTCFlowRegimeRows(result.Rows)
	result.Summary = summarizeBTCFlowRegimeAudit(result.Rows, windows)
	return result, nil
}

func normalizeBTCFlowRegimeAuditConfig(auditCfg BTCFlowRegimeAuditConfig) BTCFlowRegimeAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	out := auditCfg.HorizonDays[:0]
	seen := map[int]bool{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	sort.Ints(out)
	auditCfg.HorizonDays = out
	return auditCfg
}

func newBTCFlowRegimeAcc(horizons []int) *btcFlowRegimeAcc {
	a := &btcFlowRegimeAcc{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func finalizeBTCFlowRegimeRow(key btcFlowRegimeKey, a *btcFlowRegimeAcc, horizons []int, total int) BTCFlowRegimeAuditRow {
	row := BTCFlowRegimeAuditRow{Regime: key.regime, Bias: key.bias, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if a == nil {
		return row
	}
	row.Count = a.count
	if total > 0 {
		row.Rate = float64(a.count) / float64(total)
	}
	if a.count > 0 {
		row.AvgTrendScore = a.trendTotal / float64(a.count)
		row.AvgFlowScore = a.flowTotal / float64(a.count)
	}
	for _, h := range horizons {
		if a.count > 0 {
			row.AvgReturn[h] = a.returns[h] / float64(a.count)
			row.WinRate[h] = float64(a.wins[h]) / float64(a.count)
			row.WorstDrawdown[h] = a.worstDD[h]
		}
	}
	return row
}

func btcFlowRegimeVerdict(row BTCFlowRegimeAuditRow, horizons []int) string {
	if row.Count < 5 || len(horizons) == 0 {
		return BTCFlowRegimeVerdictLowSample
	}
	lastH := horizons[len(horizons)-1]
	ret := row.AvgReturn[lastH]
	win := row.WinRate[lastH]
	dd := row.WorstDrawdown[lastH]
	if ret > 0 && win >= 0.55 && dd > -0.12 {
		return BTCFlowRegimeVerdictCandidate
	}
	if ret <= 0 && dd < -0.10 {
		return BTCFlowRegimeVerdictReject
	}
	return BTCFlowRegimeVerdictWatch
}

func sortBTCFlowRegimeRows(rows []BTCFlowRegimeAuditRow) {
	verdictOrder := map[string]int{BTCFlowRegimeVerdictCandidate: 0, BTCFlowRegimeVerdictWatch: 1, BTCFlowRegimeVerdictReject: 2, BTCFlowRegimeVerdictLowSample: 3}
	sort.Slice(rows, func(i, j int) bool {
		if verdictOrder[rows[i].Verdict] != verdictOrder[rows[j].Verdict] {
			return verdictOrder[rows[i].Verdict] < verdictOrder[rows[j].Verdict]
		}
		if rows[i].Regime != rows[j].Regime {
			return rows[i].Regime < rows[j].Regime
		}
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Bias < rows[j].Bias
	})
}

func summarizeBTCFlowRegimeAudit(rows []BTCFlowRegimeAuditRow, windows int) string {
	if windows == 0 {
		return "BTC flow by regime audit produced no windows; not enough BTC candles."
	}
	candidates := 0
	watch := 0
	for _, row := range rows {
		switch row.Verdict {
		case BTCFlowRegimeVerdictCandidate:
			candidates++
		case BTCFlowRegimeVerdictWatch:
			watch++
		}
	}
	best := "none"
	if len(rows) > 0 {
		best = fmt.Sprintf("%s/%s", rows[0].Regime, rows[0].Bias)
	}
	return fmt.Sprintf("BTC flow by regime audit windows=%d rows=%d candidates=%d watch=%d best=%s", windows, len(rows), candidates, watch, best)
}

func btcFlowRegimeGuardRecommendation(rows []BTCFlowRegimeAuditRow) string {
	for _, row := range rows {
		if row.Bias != flow.BiasAccumulation || row.Verdict != BTCFlowRegimeVerdictReject {
			continue
		}
		if row.Regime == "PANIC_SELLING" || row.Regime == "DOWNTREND" {
			return "Research note: bullish BTC flow should not promote permission in PANIC_SELLING/DOWNTREND based on current sample."
		}
	}
	return ""
}
