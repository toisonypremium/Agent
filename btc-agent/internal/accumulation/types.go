package accumulation

import (
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type Phase string

const (
	PhaseMarkdown     Phase = "MARKDOWN"
	PhaseSweep        Phase = "LIQUIDITY_SWEEP"
	PhaseAbsorption   Phase = "SELL_ABSORPTION"
	PhaseReclaim      Phase = "RECLAIM"
	PhaseConfirmed    Phase = "ACCUMULATION_CONFIRMED"
	PhaseDistribution Phase = "DISTRIBUTION"
	PhaseInvalidated  Phase = "INVALIDATED"
)

type Evidence struct {
	Name       string  `json:"name"`
	Passed     bool    `json:"passed"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type Result struct {
	Symbol          string      `json:"symbol"`
	Phase           Phase       `json:"phase"`
	Score           float64     `json:"score"`
	DataQuality     float64     `json:"data_quality"`
	Evidence        []Evidence  `json:"evidence,omitempty"`
	HardBlockers    []string    `json:"hard_blockers,omitempty"`
	NextTrigger     string      `json:"next_trigger"`
	Support         market.Zone `json:"support"`
	Resistance      market.Zone `json:"resistance"`
	FlowBias        flow.Bias   `json:"flow_bias,omitempty"`
	SweepLow        bool        `json:"sweep_low"`
	ReclaimSupport  bool        `json:"reclaim_support"`
	FailedBreakdown bool        `json:"failed_breakdown"`
	Absorption      bool        `json:"absorption"`
	EffortVsResult  bool        `json:"effort_vs_result"`
	SupplyDryUp     bool        `json:"supply_dryup"`
	RetestHold      bool        `json:"retest_hold"`
	Distribution    bool        `json:"distribution"`
	FailedBreakout  bool        `json:"failed_breakout"`
	BullScore       float64     `json:"bull_score,omitempty"`
	BearScore       float64     `json:"bear_score,omitempty"`
}
