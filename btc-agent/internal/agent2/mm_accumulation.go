package agent2

import (
	"fmt"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type MMCase string

const (
	MMCaseNoEdge              MMCase = "NO_EDGE"
	MMCaseFallingKnife        MMCase = "FALLING_KNIFE"
	MMCaseFailedSweep         MMCase = "FAILED_SWEEP"
	MMCaseAbsorptionWatch     MMCase = "ABSORPTION_WATCH"
	MMCaseSpringReclaim       MMCase = "SPRING_RECLAIM"
	MMCaseArmedProbeCandidate MMCase = "ARMED_PROBE_CANDIDATE"
	MMCaseDistributionTrap    MMCase = "DISTRIBUTION_TRAP"
)

type MMAccumulationSignal struct {
	Symbol          string             `json:"symbol"`
	Case            MMCase             `json:"case"`
	Phase           accumulation.Phase `json:"phase,omitempty"`
	Score           float64            `json:"score"`
	Pass            bool               `json:"pass"`
	HardBlock       bool               `json:"hard_block"`
	SweepLow        bool               `json:"sweep_low"`
	ReclaimSupport  bool               `json:"reclaim_support"`
	FailedBreakdown bool               `json:"failed_breakdown"`
	Absorption      bool               `json:"absorption"`
	EffortVsResult  bool               `json:"effort_vs_result"`
	SupplyDryUp     bool               `json:"supply_dryup"`
	RetestHold      bool               `json:"retest_hold"`
	Distribution    bool               `json:"distribution"`
	FailedBreakout  bool               `json:"failed_breakout"`
	Support         market.Zone        `json:"support"`
	Resistance      market.Zone        `json:"resistance"`
	FlowBias        flow.Bias          `json:"flow_bias,omitempty"`
	BullScore       float64            `json:"bull_score,omitempty"`
	BearScore       float64            `json:"bear_score,omitempty"`
	Reasons         []string           `json:"reasons,omitempty"`
	Missing         []string           `json:"missing,omitempty"`
	NextTrigger     string             `json:"next_trigger,omitempty"`
}

func AnalyzeMMAccumulation(symbol string, candles []market.Candle) MMAccumulationSignal {
	result := accumulation.Analyze(symbol, candles)
	s := MMAccumulationSignal{
		Symbol:          symbol,
		Phase:           result.Phase,
		Case:            mmCaseFromAccumulation(result),
		Score:           result.Score,
		Pass:            accumulation.IsBullishPhase(result.Phase),
		HardBlock:       len(result.HardBlockers) > 0,
		SweepLow:        result.SweepLow,
		ReclaimSupport:  result.ReclaimSupport,
		FailedBreakdown: result.FailedBreakdown,
		Absorption:      result.Absorption,
		EffortVsResult:  result.EffortVsResult,
		SupplyDryUp:     result.SupplyDryUp,
		RetestHold:      result.RetestHold,
		Distribution:    result.Distribution,
		FailedBreakout:  result.FailedBreakout,
		Support:         result.Support,
		Resistance:      result.Resistance,
		FlowBias:        result.FlowBias,
		BullScore:       result.BullScore,
		BearScore:       result.BearScore,
		NextTrigger:     result.NextTrigger,
	}
	for _, evidence := range result.Evidence {
		if evidence.Passed {
			s.Reasons = append(s.Reasons, evidence.Reason)
		} else {
			s.Missing = append(s.Missing, evidence.Reason)
		}
	}
	if len(result.HardBlockers) > 0 {
		s.Reasons = append(s.Reasons, result.HardBlockers...)
	}
	if len(candles) < 25 {
		s.Missing = append(s.Missing, "MM footprint chưa đủ dữ liệu")
	}
	return s
}

func mmCaseFromAccumulation(result accumulation.Result) MMCase {
	switch result.Phase {
	case accumulation.PhaseInvalidated:
		return MMCaseFallingKnife
	case accumulation.PhaseDistribution:
		return MMCaseDistributionTrap
	case accumulation.PhaseSweep:
		return MMCaseFailedSweep
	case accumulation.PhaseAbsorption:
		return MMCaseAbsorptionWatch
	case accumulation.PhaseReclaim:
		return MMCaseSpringReclaim
	case accumulation.PhaseConfirmed:
		return MMCaseArmedProbeCandidate
	default:
		return MMCaseNoEdge
	}
}

func mmReason(sig MMAccumulationSignal) string {
	if len(sig.Reasons) > 0 {
		return fmt.Sprintf("MM case %s score %.0f: %s", sig.Case, sig.Score, sig.Reasons[0])
	}
	if len(sig.Missing) > 0 {
		return fmt.Sprintf("MM case %s score %.0f: %s", sig.Case, sig.Score, sig.Missing[0])
	}
	return fmt.Sprintf("MM case %s score %.0f", sig.Case, sig.Score)
}
