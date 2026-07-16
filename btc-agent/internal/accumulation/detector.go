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

// AnalyzeWithFootprint là wrapper của Analyze() bổ sung MMFootprint signal từ
// microstructure time-series. Footprint signal có thể nâng phase từ MARKDOWN
// lên SELL_ABSORPTION khi có bằng chứng MM absorption mạnh từ orderflow —
// tức là phát hiện pattern mà OHLCV chưa thấy.
//
// An toàn: footprint KHÔNG thể override HardBlockers, DISTRIBUTION,
// hoặc INVALIDATED. Chỉ hoạt động khi phase là MARKDOWN và không có hard block.
func AnalyzeWithFootprint(symbol string, candles []market.Candle, fp MMFootprint) Result {
	r := Analyze(symbol, candles)
	applyMMFootprint(&r, fp)
	return r
}

// applyMMFootprint inject MM footprint evidence vào Result đã classify.
func applyMMFootprint(r *Result, fp MMFootprint) {
	if fp.Verdict == "" || fp.Verdict == "NO_SIGNAL" {
		return
	}
	// Không can thiệp khi có hard block hoặc phase nguy hiểm
	if len(r.HardBlockers) > 0 {
		return
	}
	if r.Phase == PhaseDistribution || r.Phase == PhaseInvalidated {
		return
	}

	// Thêm footprint như một evidence item
	footprintPassed := fp.FootprintScore >= 0.40
	footprintPoints := 0.0
	if fp.Verdict == "MM_ACCUMULATING" {
		footprintPoints = 20
	} else if fp.Verdict == "POSSIBLE_ACCUMULATION" {
		footprintPoints = 12
	} else if fp.Verdict == "WATCH" {
		footprintPoints = 5
	}
	reason := "MM footprint (" + fp.Verdict + "): CVD divergence=" +
		boolStr(fp.CVDPriceDivergence) + " taker_anomaly=" + boolStr(fp.TakerBuyAnomaly) +
		" bid_streak=" + intStr(fp.BidSupportStreak)
	r.Evidence = append(r.Evidence, Evidence{
		Name:       "mm_footprint",
		Passed:     footprintPassed,
		Score:      footprintPoints,
		Confidence: fp.FootprintScore,
		Reason:     reason,
	})
	if !footprintPassed {
		return
	}
	// Cộng điểm footprint vào score
	newScore := r.Score + footprintPoints
	if newScore > 100 {
		newScore = 100
	}
	r.Score = newScore

	// Nâng phase MARKDOWN → SELL_ABSORPTION khi footprint đủ mạnh
	// Điều kiện: phase MARKDOWN + không có hard block + footprint >= POSSIBLE_ACCUMULATION
	if r.Phase == PhaseMarkdown && (fp.Verdict == "POSSIBLE_ACCUMULATION" || fp.Verdict == "MM_ACCUMULATING") {
		r.Phase = PhaseAbsorption
		r.NextTrigger = "MM footprint: sell pressure đang bị hấp thụ trên orderflow; chờ OHLCV confirm reclaim support."
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune(0+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// MMFootprint là subset của microstructure.MMFootprintSignal dùng trong accumulation.
// Tách riêng để tránh circular import.
type MMFootprint struct {
	Verdict            string  `json:"verdict"`
	FootprintScore     float64 `json:"footprint_score"`
	CVDPriceDivergence bool    `json:"cvd_price_divergence"`
	TakerBuyAnomaly    bool    `json:"taker_buy_anomaly"`
	BidSupportStreak   int     `json:"bid_support_streak"`
}
