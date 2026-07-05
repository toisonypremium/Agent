package backtest

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	AssetFlowEntryTriggerPass      = "PASS"
	AssetFlowEntryTriggerSoftFail  = "SOFT_FAIL"
	AssetFlowEntryTriggerHardBlock = "HARD_BLOCK"

	AssetFlowEntryVerdictCandidate = "CANDIDATE"
	AssetFlowEntryVerdictWatch     = "WATCH"
	AssetFlowEntryVerdictReject    = "REJECT"
	AssetFlowEntryVerdictLowSample = "LOW_SAMPLE"
)

type AssetFlowEntryAuditConfig struct {
	HorizonDays   []int    `json:"horizon_days"`
	TargetSymbols []string `json:"target_symbols"`
	MinWindow1D   int      `json:"min_window_1d"`
}

type AssetFlowEntryAuditResult struct {
	Enabled bool                     `json:"enabled"`
	Rows    []AssetFlowEntryAuditRow `json:"rows"`
	Summary string                   `json:"summary"`
}

type AssetFlowEntryAuditRow struct {
	Symbol          string          `json:"symbol"`
	FlowBias        flow.Bias       `json:"flow_bias"`
	Trigger         string          `json:"trigger"`
	BullScoreBucket string          `json:"bull_score_bucket"`
	Count           int             `json:"count"`
	AvgBullScore    float64         `json:"avg_bull_score"`
	AvgBearScore    float64         `json:"avg_bear_score"`
	AvgReturn       map[int]float64 `json:"avg_return"`
	WinRate         map[int]float64 `json:"win_rate"`
	WorstDrawdown   map[int]float64 `json:"worst_drawdown"`
	Score           float64         `json:"score"`
	Verdict         string          `json:"verdict"`
}

type assetFlowEntryAuditKey struct {
	symbol string
	bias   flow.Bias
	trig   string
	bucket string
}

type assetFlowEntryAuditAcc struct {
	count       int
	bullTotal   float64
	bearTotal   float64
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
}

func RunAssetFlowEntryAudit(cfg config.Config, assets map[string][]market.Candle, auditCfg AssetFlowEntryAuditConfig) (AssetFlowEntryAuditResult, error) {
	auditCfg = normalizeAssetFlowEntryAuditConfig(cfg, auditCfg)
	if len(auditCfg.TargetSymbols) == 0 {
		return AssetFlowEntryAuditResult{}, fmt.Errorf("no target symbols for asset flow entry audit")
	}
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	acc := map[assetFlowEntryAuditKey]*assetFlowEntryAuditAcc{}
	windows := 0
	minBull, allowNeutral := assetFlowEntryAuditParams(cfg)
	for _, sym := range auditCfg.TargetSymbols {
		candles := assets[sym]
		if len(candles) < need {
			continue
		}
		for i := auditCfg.MinWindow1D; i+maxH < len(candles); i++ {
			entry := candles[i].Close
			if entry <= 0 {
				continue
			}
			windows++
			sig := agent2.AssetFlowEntry(sym, candles[:i+1], minBull, allowNeutral)
			bias := sig.Bias
			if bias == "" {
				bias = flow.BiasNeutral
			}
			key := assetFlowEntryAuditKey{symbol: sym, bias: bias, trig: assetFlowEntryTrigger(sig), bucket: assetFlowBullScoreBucket(sig.BullScore)}
			a := acc[key]
			if a == nil {
				a = newAssetFlowEntryAuditAcc(auditCfg.HorizonDays)
				acc[key] = a
			}
			a.count++
			a.bullTotal += sig.BullScore
			a.bearTotal += sig.BearScore
			for _, h := range auditCfg.HorizonDays {
				future := candles[i+h]
				ret := (future.Close - entry) / entry
				dd := worstDrawdown(candles[i+1:i+h+1], entry)
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
	}
	result := AssetFlowEntryAuditResult{Enabled: true}
	for key, a := range acc {
		row := finalizeAssetFlowEntryAuditRow(key, a, auditCfg.HorizonDays)
		row.Score = assetFlowEntryAuditScore(row, auditCfg.HorizonDays)
		row.Verdict = assetFlowEntryAuditVerdict(row, auditCfg.HorizonDays)
		result.Rows = append(result.Rows, row)
	}
	sortAssetFlowEntryAuditRows(result.Rows)
	result.Summary = summarizeAssetFlowEntryAudit(result.Rows, windows)
	return result, nil
}

func normalizeAssetFlowEntryAuditConfig(cfg config.Config, auditCfg AssetFlowEntryAuditConfig) AssetFlowEntryAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	outH := auditCfg.HorizonDays[:0]
	seenH := map[int]bool{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seenH[h] {
			outH = append(outH, h)
			seenH[h] = true
		}
	}
	sort.Ints(outH)
	auditCfg.HorizonDays = outH
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func assetFlowEntryAuditParams(cfg config.Config) (float64, bool) {
	minBull := cfg.Risk.MinAssetFlowBullScore
	if minBull <= 0 {
		minBull = 0.25
	}
	return minBull, true
}

func assetFlowEntryTrigger(sig agent2.AssetFlowEntrySignal) string {
	switch {
	case sig.Pass:
		return AssetFlowEntryTriggerPass
	case sig.HardBlock:
		return AssetFlowEntryTriggerHardBlock
	default:
		return AssetFlowEntryTriggerSoftFail
	}
}

func assetFlowBullScoreBucket(score float64) string {
	switch {
	case score < 0.25:
		return "<0.25"
	case score < 0.50:
		return "0.25-0.50"
	default:
		return "0.50+"
	}
}

func newAssetFlowEntryAuditAcc(horizons []int) *assetFlowEntryAuditAcc {
	a := &assetFlowEntryAuditAcc{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func finalizeAssetFlowEntryAuditRow(key assetFlowEntryAuditKey, a *assetFlowEntryAuditAcc, horizons []int) AssetFlowEntryAuditRow {
	row := AssetFlowEntryAuditRow{Symbol: key.symbol, FlowBias: key.bias, Trigger: key.trig, BullScoreBucket: key.bucket, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if a == nil {
		return row
	}
	row.Count = a.count
	if a.count > 0 {
		row.AvgBullScore = a.bullTotal / float64(a.count)
		row.AvgBearScore = a.bearTotal / float64(a.count)
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

func assetFlowEntryAuditScore(row AssetFlowEntryAuditRow, horizons []int) float64 {
	if len(horizons) == 0 {
		return 0
	}
	lastH := horizons[len(horizons)-1]
	return row.AvgReturn[lastH]*100 + row.WinRate[lastH]*5 - math.Abs(row.WorstDrawdown[lastH]*100)*1.5 + row.AvgBullScore
}

func assetFlowEntryAuditVerdict(row AssetFlowEntryAuditRow, horizons []int) string {
	if row.Count < 5 || len(horizons) == 0 {
		return AssetFlowEntryVerdictLowSample
	}
	lastH := horizons[len(horizons)-1]
	if row.AvgReturn[lastH] > 0 && row.WinRate[lastH] >= 0.50 && row.WorstDrawdown[lastH] > -0.12 {
		return AssetFlowEntryVerdictCandidate
	}
	if row.AvgReturn[lastH] <= 0 && row.WorstDrawdown[lastH] < -0.10 {
		return AssetFlowEntryVerdictReject
	}
	return AssetFlowEntryVerdictWatch
}

func sortAssetFlowEntryAuditRows(rows []AssetFlowEntryAuditRow) {
	order := map[string]int{AssetFlowEntryVerdictCandidate: 0, AssetFlowEntryVerdictWatch: 1, AssetFlowEntryVerdictReject: 2, AssetFlowEntryVerdictLowSample: 3}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Symbol != rows[j].Symbol {
			return rows[i].Symbol < rows[j].Symbol
		}
		if order[rows[i].Verdict] != order[rows[j].Verdict] {
			return order[rows[i].Verdict] < order[rows[j].Verdict]
		}
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].Trigger != rows[j].Trigger {
			return rows[i].Trigger < rows[j].Trigger
		}
		return rows[i].BullScoreBucket < rows[j].BullScoreBucket
	})
}

func summarizeAssetFlowEntryAudit(rows []AssetFlowEntryAuditRow, windows int) string {
	if windows == 0 {
		return "Asset flow entry audit produced no windows; not enough asset candles."
	}
	candidates := 0
	watch := 0
	for _, row := range rows {
		switch row.Verdict {
		case AssetFlowEntryVerdictCandidate:
			candidates++
		case AssetFlowEntryVerdictWatch:
			watch++
		}
	}
	best := "none"
	if len(rows) > 0 {
		best = fmt.Sprintf("%s %s %s %s", rows[0].Symbol, rows[0].FlowBias, rows[0].Trigger, rows[0].BullScoreBucket)
	}
	return fmt.Sprintf("Asset flow entry audit windows=%d rows=%d candidates=%d watch=%d best=%s", windows, len(rows), candidates, watch, best)
}
