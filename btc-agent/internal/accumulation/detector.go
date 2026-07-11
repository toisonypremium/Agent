package accumulation

import (
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func Analyze(symbol string, candles []market.Candle) Result {
	r := Result{Symbol: symbol, Phase: PhaseMarkdown, NextTrigger: "Chờ đủ dữ liệu nến để nhận diện accumulation."}
	if len(candles) < 25 {
		r.DataQuality = dataQuality(len(candles))
		r.HardBlockers = append(r.HardBlockers, "not enough candles")
		return r
	}
	sig := flow.Analyze(candles, "1d", 60)
	r.Support = sig.Support
	r.Resistance = sig.Resistance
	r.FlowBias = sig.FlowBias
	r.BullScore = sig.BullScore
	r.BearScore = sig.BearScore
	r.SweepLow = sig.SweepLow
	r.ReclaimSupport = sig.ReclaimSupport
	r.FailedBreakdown = sig.FailedBreakdown
	r.Absorption = sig.Absorption
	r.Distribution = sig.Distribution
	r.FailedBreakout = sig.FailedBreakout
	r.EffortVsResult = effortVsResult(candles, sig.Support)
	r.SupplyDryUp = supplyDryUp(candles)
	r.RetestHold = retestHold(candles, sig.Support)
	r.DataQuality = dataQuality(len(candles))
	if fallingKnifeBreakdown(candles, sig.Support) {
		r.HardBlockers = append(r.HardBlockers, "falling knife breakdown")
	}
	score(&r)
	classify(&r)
	return r
}

func dataQuality(n int) float64 {
	switch {
	case n >= 80:
		return 1
	case n >= 60:
		return 0.9
	case n >= 40:
		return 0.75
	case n >= 25:
		return 0.6
	default:
		return 0.3
	}
}

func IsBullishPhase(p Phase) bool {
	return p == PhaseReclaim || p == PhaseConfirmed
}

func IsConfirmed(p Phase) bool {
	return p == PhaseConfirmed
}
