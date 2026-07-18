package flow

import (
	"fmt"
	"math"

	"btc-agent/internal/market"
)

func DefaultParams() Params {
	return Params{VolumeHighMultiplier: 1.20, WickRatio: 0.30, NearSupportLow: 1.025, NearSupportClose: 1.04, NearResistanceHigh: 0.975, NearResistanceClose: 0.965, AccumulationScore: 0.30, DistributionScore: 0.35, TrapScore: 0.40}
}

func (p Params) normalized() Params {
	d := DefaultParams()
	if p.VolumeHighMultiplier <= 0 {
		p.VolumeHighMultiplier = d.VolumeHighMultiplier
	}
	if p.WickRatio <= 0 {
		p.WickRatio = d.WickRatio
	}
	if p.NearSupportLow <= 0 {
		p.NearSupportLow = d.NearSupportLow
	}
	if p.NearSupportClose <= 0 {
		p.NearSupportClose = d.NearSupportClose
	}
	if p.NearResistanceHigh <= 0 {
		p.NearResistanceHigh = d.NearResistanceHigh
	}
	if p.NearResistanceClose <= 0 {
		p.NearResistanceClose = d.NearResistanceClose
	}
	if p.AccumulationScore <= 0 {
		p.AccumulationScore = d.AccumulationScore
	}
	if p.DistributionScore <= 0 {
		p.DistributionScore = d.DistributionScore
	}
	if p.TrapScore <= 0 {
		p.TrapScore = d.TrapScore
	}
	return p
}

func Analyze(c []market.Candle, timeframe string, lookback int) Signal {
	return AnalyzeWithParams(c, timeframe, lookback, DefaultParams())
}

func AnalyzeWithParams(c []market.Candle, timeframe string, lookback int, params Params) Signal {
	params = params.normalized()
	s := Signal{Timeframe: timeframe, FlowBias: BiasNeutral}
	if len(c) < 25 {
		s.Notes = append(s.Notes, "chưa đủ dữ liệu flow")
		return s
	}
	if lookback <= 0 || lookback > len(c) {
		lookback = len(c)
	}
	s.Support, s.Resistance = market.RangeZone(c[:len(c)-1], lookback-1)
	if !s.Support.Valid() || !s.Resistance.Valid() {
		s.Notes = append(s.Notes, "zone không hợp lệ")
		return s
	}

	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgVol := averageVolume(c[:len(c)-1], min(20, len(c)-1))
	if avgVol <= 0 {
		avgVol = prev.Volume
	}
	bodyHigh := math.Max(last.Open, last.Close)
	bodyLow := math.Min(last.Open, last.Close)
	rangeSize := last.High - last.Low
	lowerWick := bodyLow - last.Low
	upperWick := last.High - bodyHigh
	mid := (last.High + last.Low) / 2
	nearSupport := last.Low <= s.Support.High*params.NearSupportLow || last.Close <= s.Support.High*params.NearSupportClose
	nearResistance := last.High >= s.Resistance.Low*params.NearResistanceHigh || last.Close >= s.Resistance.Low*params.NearResistanceClose
	volumeHigh := last.Volume >= avgVol*params.VolumeHighMultiplier
	wide := rangeSize > 0
	lowerWickRatio, upperWickRatio := 0.0, 0.0
	if wide {
		lowerWickRatio = lowerWick / rangeSize
		upperWickRatio = upperWick / rangeSize
	}
	s.Diagnostics = FlowDiagnostics{NearSupport: nearSupport, NearResistance: nearResistance, VolumeHigh: volumeHigh, LowerWickRatio: lowerWickRatio, UpperWickRatio: upperWickRatio, AvgVolume: avgVol, LastVolume: last.Volume}

	s.SweepLow = last.Low < s.Support.Low || prev.Low < s.Support.Low
	s.ReclaimSupport = last.Low < s.Support.Low && last.Close >= s.Support.Low
	s.FailedBreakdown = last.Low < s.Support.Low && last.Close >= s.Support.High
	s.SweepHigh = last.High > s.Resistance.High || prev.High > s.Resistance.High
	s.RejectResistance = last.High > s.Resistance.High && last.Close <= s.Resistance.High
	s.FailedBreakout = last.High > s.Resistance.High && last.Close <= s.Resistance.Low

	if wide && volumeHigh && nearSupport && lowerWick >= rangeSize*params.WickRatio && last.Close >= mid {
		s.Absorption = true
	}
	if wide && volumeHigh && nearResistance && upperWick >= rangeSize*params.WickRatio && last.Close <= mid {
		s.Distribution = true
	}

	s.scoreAndBias(params)
	return s
}

func AnalyzeMultiFrame(btc map[string][]market.Candle) MultiFrame {
	return AnalyzeMultiFrameWithParams(btc, DefaultParams())
}

func AnalyzeMultiFrameWithParams(btc map[string][]market.Candle, params Params) MultiFrame {
	params = params.normalized()
	m := MultiFrame{
		Daily:    AnalyzeWithParams(btc["1d"], "1d", 60, params),
		FourHour: AnalyzeWithParams(btc["4h"], "4h", 80, params),
		Weekly:   AnalyzeWithParams(btc["1w"], "1w", 52, params),
		Bias:     BiasNeutral,
	}
	m.Score = clamp01(m.Daily.Confidence*0.55 + m.FourHour.Confidence*0.30 + m.Weekly.Confidence*0.15)
	m.Bias = aggregateBias(m.Daily, m.FourHour, m.Weekly)
	m.Summary = summarize(m)
	return m
}

func (s *Signal) scoreAndBias(params Params) {
	bull, bear := 0.0, 0.0
	add := func(name string, pass bool, bullWeight, bearWeight float64, reason string) {
		s.Components = append(s.Components, FlowComponent{Name: name, Pass: pass, Bull: boolWeight(pass, bullWeight), Bear: boolWeight(pass, bearWeight), Reason: reason})
		if !pass {
			return
		}
		if bullWeight > 0 {
			bull += bullWeight
		}
		if bearWeight > 0 {
			bear += bearWeight
		}
		s.Notes = append(s.Notes, reason)
	}
	add("sweep_low", s.SweepLow, 0.10, 0, "quét thanh khoản dưới hỗ trợ")
	add("reclaim_support", s.ReclaimSupport, 0.20, 0, "đóng nến reclaim hỗ trợ")
	add("failed_breakdown", s.FailedBreakdown, 0.20, 0, "failed breakdown")
	add("absorption", s.Absorption, 0.25, 0, "volume bán được hấp thụ gần hỗ trợ")
	add("sweep_high", s.SweepHigh, 0, 0.10, "quét thanh khoản trên kháng cự")
	add("reject_resistance", s.RejectResistance, 0, 0.20, "bị từ chối ở kháng cự")
	add("failed_breakout", s.FailedBreakout, 0, 0.25, "failed breakout / bull trap")
	add("distribution", s.Distribution, 0, 0.25, "volume mua bị xả gần kháng cự")

	s.BullScore = clamp01(bull)
	s.BearScore = clamp01(bear)
	s.Diagnostics.NeedBullScore = clamp01(math.Max(0, params.AccumulationScore-s.BullScore))
	s.Diagnostics.NeedBearScore = clamp01(math.Max(0, params.DistributionScore-s.BearScore))
	s.Diagnostics.NextBullTrigger = nextBullTrigger(*s)
	if bear >= params.TrapScore && s.FailedBreakout {
		s.FlowBias = BiasBullTrap
		s.Confidence = clamp01(bear)
		return
	}
	if bear >= params.DistributionScore {
		s.FlowBias = BiasDistribution
		s.Confidence = clamp01(bear)
		return
	}
	if bull >= params.TrapScore && s.FailedBreakdown {
		s.FlowBias = BiasBearTrap
		s.Confidence = clamp01(bull)
		return
	}
	if bull >= params.AccumulationScore {
		s.FlowBias = BiasAccumulation
		s.Confidence = clamp01(bull)
		return
	}
	s.FlowBias = BiasNeutral
	s.Confidence = clamp01(math.Max(bull, bear))
	if len(s.Notes) == 0 {
		s.Notes = append(s.Notes, "chưa thấy liquidity flow rõ")
	}
}

func boolWeight(pass bool, weight float64) float64 {
	if !pass {
		return 0
	}
	return weight
}

func nextBullTrigger(s Signal) string {
	if !s.SweepLow {
		return "chờ sweep low dưới support"
	}
	if !s.ReclaimSupport {
		return "chờ close reclaim support"
	}
	if !s.Absorption && s.Diagnostics.NearSupport {
		return "chờ absorption/volume bán giảm gần support"
	}
	if !s.FailedBreakdown {
		return "chờ failed breakdown rõ"
	}
	return "cần thêm bull component để flow >= ngưỡng"
}

func aggregateBias(daily, h4, weekly Signal) Bias {
	bullTrap := weightedBias(daily, h4, weekly, BiasBullTrap)
	distribution := weightedBias(daily, h4, weekly, BiasDistribution)
	bearTrap := weightedBias(daily, h4, weekly, BiasBearTrap)
	accumulation := weightedBias(daily, h4, weekly, BiasAccumulation)
	if daily.FlowBias == BiasBullTrap || bullTrap >= 0.45 {
		return BiasBullTrap
	}
	if daily.FlowBias == BiasDistribution || distribution >= 0.40 {
		return BiasDistribution
	}
	weeklyBearish := weekly.FlowBias == BiasBullTrap || weekly.FlowBias == BiasDistribution
	lowerTimeframeBullish := daily.FlowBias == BiasAccumulation || daily.FlowBias == BiasBearTrap || h4.FlowBias == BiasAccumulation || h4.FlowBias == BiasBearTrap
	if weeklyBearish && lowerTimeframeBullish {
		return BiasNeutral
	}
	if daily.FlowBias == BiasBearTrap || bearTrap >= 0.45 {
		return BiasBearTrap
	}
	if daily.FlowBias == BiasAccumulation || accumulation >= 0.35 {
		return BiasAccumulation
	}
	return BiasNeutral
}

func weightedBias(daily, h4, weekly Signal, b Bias) float64 {
	score := 0.0
	if daily.FlowBias == b {
		score += daily.Confidence * 0.55
	}
	if h4.FlowBias == b {
		score += h4.Confidence * 0.30
	}
	if weekly.FlowBias == b {
		score += weekly.Confidence * 0.15
	}
	return score
}

func summarize(m MultiFrame) string {
	switch m.Bias {
	case BiasBullTrap:
		return "Flow nghiêng về bull trap/distribution; không đuổi breakout nóng."
	case BiasDistribution:
		return "Flow nghiêng về phân phối gần kháng cự; ưu tiên bảo toàn vốn."
	case BiasBearTrap:
		return "Có dấu hiệu bear trap/failed breakdown; chỉ ARMED nếu risk khác không cao."
	case BiasAccumulation:
		return "Có dấu hiệu hấp thụ hoặc reclaim; theo dõi pullback thay vì bắt dao rơi."
	default:
		return fmt.Sprintf("Flow chưa rõ; daily=%s 4h=%s weekly=%s.", m.Daily.FlowBias, m.FourHour.FlowBias, m.Weekly.FlowBias)
	}
}

func averageVolume(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	for _, x := range c[start:] {
		sum += x.Volume
	}
	return sum / float64(n)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
