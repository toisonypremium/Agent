package accumulation

import (
	"math"

	"btc-agent/internal/flow"
)

func score(r *Result) {
	s := 0.0
	add := func(name string, passed bool, points float64, reason string) {
		componentScore := 0.0
		confidence := 0.0
		if passed {
			componentScore = points
			confidence = points / 100
			s += points
		}
		r.Evidence = append(r.Evidence, Evidence{Name: name, Passed: passed, Score: componentScore, Confidence: confidence, Reason: reason})
	}
	add("expected_zone", r.Support.Valid(), 15, "support/discount zone hợp lệ")
	add("liquidity_sweep", r.SweepLow, 20, "low quét dưới support/swing low")
	add("sell_absorption", r.Absorption || r.EffortVsResult, 20, "volume bán được hấp thụ hoặc effort/result tốt")
	add("cvd_divergence_proxy", r.FailedBreakdown, 15, "failed breakdown OHLCV proxy; CVD thật chưa có")
	add("oi_liquidation_reset_proxy", r.SupplyDryUp, 10, "supply dry-up OHLCV proxy; OI/liquidation thật chưa có")
	add("reclaim", r.ReclaimSupport, 10, "close reclaim support")
	add("retest_hold", r.RetestHold, 10, "retest giữ support")
	if r.FlowBias == flow.BiasDistribution || r.FlowBias == flow.BiasBullTrap || r.Distribution || r.FailedBreakout {
		s -= 30
	}
	if s < 0 {
		s = 0
	}
	if s > 100 {
		s = 100
	}
	r.Score = math.Round(s*10) / 10
}

func classify(r *Result) {
	if r.DataQuality < 0.6 {
		r.Phase = PhaseMarkdown
		r.NextTrigger = "Chờ đủ dữ liệu đã đóng để xác nhận accumulation."
		return
	}
	if r.FlowBias == flow.BiasDistribution || r.FlowBias == flow.BiasBullTrap || r.Distribution || r.FailedBreakout {
		r.Phase = PhaseDistribution
		r.HardBlockers = append(r.HardBlockers, "distribution/bull trap")
		r.NextTrigger = "Chờ hết distribution/bull-trap và reclaim lại support với bull flow."
		return
	}
	if len(r.HardBlockers) > 0 {
		r.Phase = PhaseInvalidated
		r.NextTrigger = "Chờ ngừng lower-low, close reclaim support và volume bán giảm."
		return
	}
	if r.Score >= 75 && r.RetestHold && r.ReclaimSupport {
		r.Phase = PhaseConfirmed
		r.NextTrigger = "BTC accumulation confirmed; vẫn cần risk/FOMO/RR/data gates pass trước ALLOWED."
		return
	}
	if r.ReclaimSupport && (r.Absorption || r.EffortVsResult || r.FailedBreakdown) {
		r.Phase = PhaseReclaim
		r.NextTrigger = "Chờ retest giữ support và score >= 75 để xác nhận accumulation."
		return
	}
	if r.Absorption || r.EffortVsResult {
		r.Phase = PhaseAbsorption
		r.NextTrigger = "Chờ close reclaim support và retest giữ vùng."
		return
	}
	if r.SweepLow {
		r.Phase = PhaseSweep
		r.NextTrigger = "Chờ close reclaim support sau sweep, tránh bắt dao rơi."
		return
	}
	r.Phase = PhaseMarkdown
	r.NextTrigger = "Chờ sweep low + close reclaim support + retest giữ vùng."
}
