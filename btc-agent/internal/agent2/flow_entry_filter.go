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
	Reason          string    `json:"reason"`
}

func AssetFlowEntry(sym string, candles []market.Candle, minBullScore float64, allowNeutralReclaim bool) AssetFlowEntrySignal {
	s := AssetFlowEntrySignal{Symbol: sym, Bias: flow.BiasNeutral, Reason: "asset flow entry chưa đủ dữ liệu"}
	if len(candles) < 25 {
		return s
	}
	if minBullScore <= 0 {
		minBullScore = 0.25
	}
	sig := flow.Analyze(candles, "1d", 60)
	s.Bias = sig.FlowBias
	s.BullScore = sig.BullScore
	s.BearScore = sig.BearScore
	s.SweepLow = sig.SweepLow
	s.ReclaimSupport = sig.ReclaimSupport
	s.FailedBreakdown = sig.FailedBreakdown
	s.Absorption = sig.Absorption
	s.FailedBreakout = sig.FailedBreakout
	s.Distribution = sig.Distribution

	if sig.FlowBias == flow.BiasBullTrap || sig.FlowBias == flow.BiasDistribution || sig.FailedBreakout || sig.Distribution {
		s.HardBlock = true
		s.Reason = fmt.Sprintf("asset flow entry chặn: bias=%s bull=%.2f bear=%.2f failed_breakout=%v distribution=%v", sig.FlowBias, sig.BullScore, sig.BearScore, sig.FailedBreakout, sig.Distribution)
		return s
	}
	if sig.FlowBias == flow.BiasAccumulation || sig.FlowBias == flow.BiasBearTrap || sig.FailedBreakdown || sig.Absorption && sig.BullScore >= minBullScore || allowNeutralReclaim && sig.SweepLow && sig.ReclaimSupport {
		s.Pass = true
		s.Reason = fmt.Sprintf("asset flow entry OK: bias=%s bull=%.2f sweep=%v reclaim=%v absorption=%v failed_breakdown=%v", sig.FlowBias, sig.BullScore, sig.SweepLow, sig.ReclaimSupport, sig.Absorption, sig.FailedBreakdown)
		return s
	}
	s.Reason = fmt.Sprintf("asset flow entry chưa xác nhận: bias=%s bull=%.2f bear=%.2f", sig.FlowBias, sig.BullScore, sig.BearScore)
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
