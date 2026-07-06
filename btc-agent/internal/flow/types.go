package flow

import "btc-agent/internal/market"

type Bias string

const (
	BiasNeutral      Bias = "NEUTRAL"
	BiasAccumulation Bias = "ACCUMULATION"
	BiasDistribution Bias = "DISTRIBUTION"
	BiasBullTrap     Bias = "BULL_TRAP"
	BiasBearTrap     Bias = "BEAR_TRAP"
)

type Params struct {
	VolumeHighMultiplier float64 `json:"volume_high_multiplier"`
	WickRatio            float64 `json:"wick_ratio"`
	NearSupportLow       float64 `json:"near_support_low"`
	NearSupportClose     float64 `json:"near_support_close"`
	NearResistanceHigh   float64 `json:"near_resistance_high"`
	NearResistanceClose  float64 `json:"near_resistance_close"`
	AccumulationScore    float64 `json:"accumulation_score"`
	DistributionScore    float64 `json:"distribution_score"`
	TrapScore            float64 `json:"trap_score"`
}

type FlowComponent struct {
	Name   string  `json:"name"`
	Bull   float64 `json:"bull,omitempty"`
	Bear   float64 `json:"bear,omitempty"`
	Pass   bool    `json:"pass"`
	Reason string  `json:"reason,omitempty"`
}

type FlowDiagnostics struct {
	NearSupport     bool    `json:"near_support"`
	NearResistance  bool    `json:"near_resistance"`
	VolumeHigh      bool    `json:"volume_high"`
	LowerWickRatio  float64 `json:"lower_wick_ratio,omitempty"`
	UpperWickRatio  float64 `json:"upper_wick_ratio,omitempty"`
	AvgVolume       float64 `json:"avg_volume,omitempty"`
	LastVolume      float64 `json:"last_volume,omitempty"`
	NeedBullScore   float64 `json:"need_bull_score,omitempty"`
	NeedBearScore   float64 `json:"need_bear_score,omitempty"`
	NextBullTrigger string  `json:"next_bull_trigger,omitempty"`
}

type Signal struct {
	Timeframe        string          `json:"timeframe"`
	Support          market.Zone     `json:"support"`
	Resistance       market.Zone     `json:"resistance"`
	SweepLow         bool            `json:"sweep_low"`
	SweepHigh        bool            `json:"sweep_high"`
	ReclaimSupport   bool            `json:"reclaim_support"`
	RejectResistance bool            `json:"reject_resistance"`
	FailedBreakdown  bool            `json:"failed_breakdown"`
	FailedBreakout   bool            `json:"failed_breakout"`
	Absorption       bool            `json:"absorption"`
	Distribution     bool            `json:"distribution"`
	BullScore        float64         `json:"bull_score"`
	BearScore        float64         `json:"bear_score"`
	FlowBias         Bias            `json:"flow_bias"`
	Confidence       float64         `json:"confidence"`
	Components       []FlowComponent `json:"components,omitempty"`
	Diagnostics      FlowDiagnostics `json:"diagnostics,omitempty"`
	Notes            []string        `json:"notes"`
}

type MultiFrame struct {
	Daily    Signal  `json:"daily"`
	FourHour Signal  `json:"four_hour"`
	Weekly   Signal  `json:"weekly"`
	Bias     Bias    `json:"bias"`
	Score    float64 `json:"score"`
	Summary  string  `json:"summary"`
}
