package backtest

import (
	"fmt"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/market"
)

type AccumulationWalkForwardSplit struct {
	SplitIndex int                          `json:"split_index"`
	EvalStart  int                          `json:"eval_start"`
	EvalEnd    int                          `json:"eval_end"`
	Embargo    int                          `json:"embargo"`
	Confirmed  AccumulationPhaseAuditRow    `json:"confirmed"`
	Audit      AccumulationPhaseAuditResult `json:"audit"`
}

type AccumulationWalkForwardReport struct {
	Enabled                bool                           `json:"enabled"`
	Splits                 []AccumulationWalkForwardSplit `json:"splits"`
	EvaluationSamples      int                            `json:"evaluation_samples"`
	WeightedFalsePositive  float64                        `json:"weighted_false_positive"`
	WorstMAE               float64                        `json:"worst_mae"`
	Status                 string                         `json:"status"`
	SizingExpansionAllowed bool                           `json:"sizing_expansion_allowed"`
	Summary                string                         `json:"summary"`
}

// RunAccumulationWalkForward evaluates confirmed accumulation only in embargoed
// evaluation windows. It never changes production config or permits sizing.
func RunAccumulationWalkForward(symbol string, daily []market.Candle, horizons []int, splitCount int, trainPct float64, embargo int) (AccumulationWalkForwardReport, error) {
	cfg := normalizeConfig(Config{MinWindow1D: 60, HorizonDays: horizons})
	if splitCount <= 0 || trainPct <= 0 || trainPct >= 1 {
		return AccumulationWalkForwardReport{}, fmt.Errorf("invalid accumulation walk-forward parameters")
	}
	if embargo < 1 {
		embargo = 1
	}
	maxH := maxHorizon(cfg.HorizonDays)
	chunk := len(daily) / splitCount
	if chunk < cfg.MinWindow1D+maxH+1 {
		return AccumulationWalkForwardReport{}, fmt.Errorf("not enough candles per accumulation walk-forward split")
	}
	out := AccumulationWalkForwardReport{Enabled: true, Status: "INSUFFICIENT_DATA", WorstMAE: 0}
	falsePositiveCount := 0.0
	for i := 0; i < splitCount; i++ {
		start := i * chunk
		end := start + chunk
		if i == splitCount-1 {
			end = len(daily)
		}
		trainEnd := start + int(float64(end-start)*trainPct)
		evalStart := trainEnd + embargo
		evalEnd := end - maxH
		if evalStart >= evalEnd {
			continue
		}
		audit := runAccumulationPhaseAuditRange(symbol, daily, cfg, evalStart, evalEnd)
		confirmed := accumulationRow(audit.Rows, accumulation.PhaseConfirmed)
		out.Splits = append(out.Splits, AccumulationWalkForwardSplit{SplitIndex: i, EvalStart: evalStart, EvalEnd: evalEnd, Embargo: embargo, Confirmed: confirmed, Audit: audit})
		out.EvaluationSamples += confirmed.Count
		falsePositiveCount += confirmed.FalsePositiveRate * float64(confirmed.Count)
		for _, value := range confirmed.WorstMAE {
			if value < out.WorstMAE {
				out.WorstMAE = value
			}
		}
	}
	if len(out.Splits) == 0 {
		return AccumulationWalkForwardReport{}, fmt.Errorf("no valid accumulation walk-forward splits")
	}
	if out.EvaluationSamples > 0 {
		out.WeightedFalsePositive = falsePositiveCount / float64(out.EvaluationSamples)
	}
	if out.EvaluationSamples >= 30 && out.WeightedFalsePositive <= 0.35 && out.WorstMAE >= -0.12 {
		out.Status = "RESEARCH_REVIEW_REQUIRED"
	}
	out.SizingExpansionAllowed = false
	out.Summary = fmt.Sprintf("accumulation walk-forward splits=%d eval_confirmed=%d false_positive=%.1f%% worst_mae=%.1f%% status=%s research-only", len(out.Splits), out.EvaluationSamples, out.WeightedFalsePositive*100, out.WorstMAE*100, out.Status)
	return out, nil
}

func accumulationRow(rows []AccumulationPhaseAuditRow, phase accumulation.Phase) AccumulationPhaseAuditRow {
	for _, row := range rows {
		if row.Phase == phase {
			return row
		}
	}
	return AccumulationPhaseAuditRow{Phase: phase, Verdict: "NO_SAMPLE"}
}
