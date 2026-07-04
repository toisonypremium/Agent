package backtest

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	FlowParamQualityKeepCurrent   = "KEEP_CURRENT"
	FlowParamQualityCandidateTune = "CANDIDATE_TUNE"
	FlowParamQualityRejectNoisy   = "REJECT_NOISY"
	FlowParamQualityNeedMoreData  = "NEED_MORE_DATA"
)

type FlowParamQualityAuditConfig struct {
	MinWindow1D       int   `json:"min_window_1d"`
	HorizonDays       []int `json:"horizon_days"`
	PrimaryHorizonDay int   `json:"primary_horizon_day"`
	MinSignals        int   `json:"min_signals"`
}

type FlowParamQualityAuditResult struct {
	Enabled bool                       `json:"enabled"`
	Rows    []FlowParamQualityAuditRow `json:"rows"`
	Summary string                     `json:"summary"`
}

type FlowParamQualityAuditRow struct {
	Name                 string          `json:"name"`
	Params               flow.Params     `json:"params"`
	Windows              int             `json:"windows"`
	BullishCount         int             `json:"bullish_count"`
	BearishCount         int             `json:"bearish_count"`
	AddedBullishCount    int             `json:"added_bullish_count"`
	AddedBearishCount    int             `json:"added_bearish_count"`
	BullishAvgReturn     map[int]float64 `json:"bullish_avg_return"`
	BullishWinRate       map[int]float64 `json:"bullish_win_rate"`
	BullishWorstDrawdown map[int]float64 `json:"bullish_worst_drawdown"`
	BearishAvgReturn     map[int]float64 `json:"bearish_avg_return"`
	BearishWinRate       map[int]float64 `json:"bearish_win_rate"`
	BearishWorstDrawdown map[int]float64 `json:"bearish_worst_drawdown"`
	AddedAvgReturn       map[int]float64 `json:"added_avg_return"`
	AddedWinRate         map[int]float64 `json:"added_win_rate"`
	AddedWorstDrawdown   map[int]float64 `json:"added_worst_drawdown"`
	FalsePositiveRate    float64         `json:"false_positive_rate"`
	DeepDrawdownRate     float64         `json:"deep_drawdown_rate"`
	Score                float64         `json:"score"`
	Verdict              string          `json:"verdict"`
}

type flowParamQualityAcc struct {
	windows              int
	bullish              *btcFlowAcc
	bearish              *btcFlowAcc
	addedBullish         *btcFlowAcc
	bullishFalsePositive int
	bullishDeepDrawdown  int
	addedBearishCount    int
}

func RunFlowParamQualityAudit(btc map[string][]market.Candle, auditCfg FlowParamQualityAuditConfig) (FlowParamQualityAuditResult, error) {
	auditCfg = normalizeFlowParamQualityAuditConfig(auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return FlowParamQualityAuditResult{}, fmt.Errorf("not enough BTC 1d candles for flow param quality audit; need %d got %d", need, len(btc1d))
	}

	sets := btcFlowParamSets()
	accs := make([]*flowParamQualityAcc, len(sets))
	for i := range sets {
		accs[i] = newFlowParamQualityAcc(auditCfg.HorizonDays)
	}
	baseParams := flow.DefaultParams()

	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		entry := btc1d[i].Close
		if entry <= 0 {
			continue
		}
		baseSig := flow.AnalyzeWithParams(btc1d[:i+1], "1d", 60, baseParams)
		baseBullish := flowSignalBullish(baseSig)
		baseBearish := flowSignalBearish(baseSig)
		for j, set := range sets {
			sig := flow.AnalyzeWithParams(btc1d[:i+1], "1d", 60, set.params)
			accs[j].windows++
			bullish := flowSignalBullish(sig)
			bearish := flowSignalBearish(sig)
			if bullish {
				accumulateBTCFlowSignal(accs[j].bullish, sig, btc1d, i, auditCfg.HorizonDays)
				primaryRet := (btc1d[i+auditCfg.PrimaryHorizonDay].Close - entry) / entry
				primaryDD := worstDrawdown(btc1d[i+1:i+auditCfg.PrimaryHorizonDay+1], entry)
				if primaryRet < 0 || primaryDD <= -0.08 {
					accs[j].bullishFalsePositive++
				}
				if primaryDD <= -0.08 {
					accs[j].bullishDeepDrawdown++
				}
				if !baseBullish {
					accumulateBTCFlowSignal(accs[j].addedBullish, sig, btc1d, i, auditCfg.HorizonDays)
				}
			}
			if bearish {
				accumulateBTCFlowSignal(accs[j].bearish, sig, btc1d, i, auditCfg.HorizonDays)
				if !baseBearish {
					accs[j].addedBearishCount++
				}
			}
		}
	}

	result := FlowParamQualityAuditResult{Enabled: true}
	for i, set := range sets {
		result.Rows = append(result.Rows, finalizeFlowParamQualityRow(set, accs[i], auditCfg.HorizonDays, auditCfg.PrimaryHorizonDay))
	}
	for i := range result.Rows {
		var current FlowParamQualityAuditRow
		if len(result.Rows) > 0 {
			current = result.Rows[0]
		}
		result.Rows[i].Verdict = flowParamQualityVerdict(result.Rows[i], current, auditCfg)
	}
	result.Summary = summarizeFlowParamQualityAudit(result.Rows)
	return result, nil
}

func normalizeFlowParamQualityAuditConfig(auditCfg FlowParamQualityAuditConfig) FlowParamQualityAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	if auditCfg.PrimaryHorizonDay <= 0 {
		auditCfg.PrimaryHorizonDay = 7
	}
	if auditCfg.MinSignals <= 0 {
		auditCfg.MinSignals = 8
	}
	seen := map[int]bool{}
	out := []int{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	if !seen[auditCfg.PrimaryHorizonDay] {
		out = append(out, auditCfg.PrimaryHorizonDay)
	}
	sort.Ints(out)
	auditCfg.HorizonDays = out
	return auditCfg
}

func newFlowParamQualityAcc(horizons []int) *flowParamQualityAcc {
	return &flowParamQualityAcc{bullish: newBTCFlowAcc(horizons), bearish: newBTCFlowAcc(horizons), addedBullish: newBTCFlowAcc(horizons)}
}

func flowSignalBullish(sig flow.Signal) bool {
	return sig.FlowBias == flow.BiasAccumulation || sig.FlowBias == flow.BiasBearTrap
}

func flowSignalBearish(sig flow.Signal) bool {
	return sig.FlowBias == flow.BiasDistribution || sig.FlowBias == flow.BiasBullTrap
}

func finalizeFlowParamQualityRow(set btcFlowParamSet, acc *flowParamQualityAcc, horizons []int, primary int) FlowParamQualityAuditRow {
	row := FlowParamQualityAuditRow{
		Name:                 set.name,
		Params:               set.params,
		BullishAvgReturn:     map[int]float64{},
		BullishWinRate:       map[int]float64{},
		BullishWorstDrawdown: map[int]float64{},
		BearishAvgReturn:     map[int]float64{},
		BearishWinRate:       map[int]float64{},
		BearishWorstDrawdown: map[int]float64{},
		AddedAvgReturn:       map[int]float64{},
		AddedWinRate:         map[int]float64{},
		AddedWorstDrawdown:   map[int]float64{},
	}
	if acc == nil {
		return row
	}
	row.Windows = acc.windows
	row.BullishCount = acc.bullish.count
	row.BearishCount = acc.bearish.count
	row.AddedBullishCount = acc.addedBullish.count
	row.AddedBearishCount = acc.addedBearishCount
	finalizeBTCFlowRow(acc.bullish, horizons, acc.windows, &row.BullishCount, new(float64), new(float64), new(float64), new(float64), row.BullishAvgReturn, row.BullishWinRate, row.BullishWorstDrawdown)
	finalizeBTCFlowRow(acc.bearish, horizons, acc.windows, &row.BearishCount, new(float64), new(float64), new(float64), new(float64), row.BearishAvgReturn, row.BearishWinRate, row.BearishWorstDrawdown)
	finalizeBTCFlowRow(acc.addedBullish, horizons, acc.windows, &row.AddedBullishCount, new(float64), new(float64), new(float64), new(float64), row.AddedAvgReturn, row.AddedWinRate, row.AddedWorstDrawdown)
	if row.BullishCount > 0 {
		row.FalsePositiveRate = float64(acc.bullishFalsePositive) / float64(row.BullishCount)
		row.DeepDrawdownRate = float64(acc.bullishDeepDrawdown) / float64(row.BullishCount)
	}
	row.Score = flowParamQualityScore(row, primary)
	return row
}

func flowParamQualityScore(row FlowParamQualityAuditRow, primary int) float64 {
	return row.BullishAvgReturn[primary] + row.BullishWinRate[primary]*0.02 - row.FalsePositiveRate*0.05 - math.Abs(row.BullishWorstDrawdown[primary])*0.50
}

func flowParamQualityVerdict(row, current FlowParamQualityAuditRow, auditCfg FlowParamQualityAuditConfig) string {
	if row.Name == "current" {
		if row.BullishCount < auditCfg.MinSignals {
			return FlowParamQualityNeedMoreData
		}
		return FlowParamQualityKeepCurrent
	}
	if row.BullishCount < auditCfg.MinSignals {
		return FlowParamQualityNeedMoreData
	}
	if row.FalsePositiveRate > 0.60 || row.DeepDrawdownRate > 0.35 {
		return FlowParamQualityRejectNoisy
	}
	primary := auditCfg.PrimaryHorizonDay
	if row.AddedBullishCount > 0 && row.BullishAvgReturn[primary] >= current.BullishAvgReturn[primary] && row.FalsePositiveRate <= current.FalsePositiveRate+0.10 && row.BullishWorstDrawdown[primary] >= current.BullishWorstDrawdown[primary]-0.03 {
		return FlowParamQualityCandidateTune
	}
	return FlowParamQualityKeepCurrent
}

func summarizeFlowParamQualityAudit(rows []FlowParamQualityAuditRow) string {
	if len(rows) == 0 {
		return "Flow param quality audit produced no rows; not enough BTC candles."
	}
	current := rows[0]
	best := current
	for _, row := range rows {
		if row.Verdict == FlowParamQualityCandidateTune && (best.Name == "current" || row.Score > best.Score) {
			best = row
		}
	}
	if best.Name != "current" {
		return fmt.Sprintf("Flow param quality audit rows=%d best=%s verdict=%s current_bullish=%d candidate_bullish=%d added=%d false_positive=%.1f%%", len(rows), best.Name, best.Verdict, current.BullishCount, best.BullishCount, best.AddedBullishCount, best.FalsePositiveRate*100)
	}
	topCandidate := current
	for _, row := range rows[1:] {
		if topCandidate.Name == "current" || row.Score > topCandidate.Score {
			topCandidate = row
		}
	}
	return fmt.Sprintf("Flow param quality audit rows=%d best=current verdict=%s current_bullish=%d top_candidate=%s candidate_verdict=%s", len(rows), current.Verdict, current.BullishCount, topCandidate.Name, topCandidate.Verdict)
}
