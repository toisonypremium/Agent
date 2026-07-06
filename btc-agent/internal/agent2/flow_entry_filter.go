package agent2

import (
	"fmt"

	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type AssetFlowEntrySignal struct {
	Symbol          string    `json:"symbol"`
	Pass            bool      `json:"pass"`
	HardBlock       bool      `json:"hard_block"`
	Bias            flow.Bias `json:"bias"`
	BullScore       float64   `json:"bull_score"`
	BearScore       float64   `json:"bear_score"`
	SweepLow        bool      `json:"sweep_low"`
	ReclaimSupport  bool      `json:"reclaim_support"`
	FailedBreakdown bool      `json:"failed_breakdown"`
	Absorption      bool      `json:"absorption"`
	FailedBreakout  bool      `json:"failed_breakout"`
	Distribution    bool      `json:"distribution"`
	MMCase          MMCase    `json:"mm_case,omitempty"`
	MMScore         float64   `json:"mm_score,omitempty"`
	MMReasons       []string  `json:"mm_reasons,omitempty"`
	MMMissing       []string  `json:"mm_missing,omitempty"`
	NextTrigger     string    `json:"next_trigger,omitempty"`
	Reason          string    `json:"reason"`
}

func AssetFlowEntry(sym string, candles []market.Candle, minBullScore float64, allowNeutralReclaim bool) AssetFlowEntrySignal {
	if len(candles) < 25 {
		return AssetFlowEntrySignal{Symbol: sym, Bias: flow.BiasNeutral, Reason: "asset flow entry chưa đủ dữ liệu"}
	}
	return AssetFlowEntryFromMM(AnalyzeMMAccumulation(sym, candles), minBullScore, allowNeutralReclaim)
}

func AssetFlowEntryFromMM(mm MMAccumulationSignal, minBullScore float64, allowNeutralReclaim bool) AssetFlowEntrySignal {
	s := AssetFlowEntrySignal{Symbol: mm.Symbol, Bias: mm.FlowBias, BullScore: mm.BullScore, BearScore: mm.BearScore, SweepLow: mm.SweepLow, ReclaimSupport: mm.ReclaimSupport, FailedBreakdown: mm.FailedBreakdown, Absorption: mm.Absorption, FailedBreakout: mm.FailedBreakout, Distribution: mm.Distribution, MMCase: mm.Case, MMScore: mm.Score, MMReasons: mm.Reasons, MMMissing: mm.Missing, NextTrigger: mm.NextTrigger, Reason: "asset flow entry chưa đủ dữ liệu"}
	if minBullScore <= 0 {
		minBullScore = 0.25
	}
	if mm.HardBlock {
		s.HardBlock = true
		s.Reason = fmt.Sprintf("asset flow entry chặn: %s", mmReason(mm))
		return s
	}
	if mm.Pass || mm.Case == MMCaseSpringReclaim || mm.Case == MMCaseArmedProbeCandidate || mm.FlowBias == flow.BiasAccumulation || mm.FlowBias == flow.BiasBearTrap || mm.FailedBreakdown || mm.Absorption && mm.BullScore >= minBullScore || allowNeutralReclaim && mm.SweepLow && mm.ReclaimSupport {
		s.Pass = true
		s.Reason = fmt.Sprintf("asset flow entry OK: %s", mmReason(mm))
		return s
	}
	s.Reason = fmt.Sprintf("asset flow entry chưa xác nhận: %s", mmReason(mm))
	return s
}

func assetFlowEntryParams(cfg config.Config) (bool, float64, bool) {
	if cfg.Risk.DisableAssetFlowEntryFilter {
		return false, 0, false
	}
	minBullScore := cfg.Risk.MinAssetFlowBullScore
	if minBullScore <= 0 {
		minBullScore = 0.25
	}
	allowNeutralReclaim := true
	if cfg.Risk.AllowNeutralReclaimEntry {
		allowNeutralReclaim = true
	}
	return true, minBullScore, allowNeutralReclaim
}
