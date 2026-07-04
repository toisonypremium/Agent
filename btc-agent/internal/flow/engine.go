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
	if s.SweepLow {
		bull += 0.10
		s.Notes = append(s.Notes, "quét thanh khoản dưới hỗ trợ")
	}
	if s.ReclaimSupport {
		bull += 0.20
		s.Notes = append(s.Notes, "đóng nến reclaim hỗ trợ")
	}
	if s.FailedBreakdown {
		bull += 0.20
		s.Notes = append(s.Notes, "failed breakdown")
	}
	if s.Absorption {
		bull += 0.25
		s.Notes = append(s.Notes, "volume bán được hấp thụ gần hỗ trợ")
	}
	if s.SweepHigh {
		bear += 0.10
		s.Notes = append(s.Notes, "quét thanh khoản trên kháng cự")
	}
	if s.RejectResistance {
		bear += 0.20
		s.Notes = append(s.Notes, "bị từ chối ở kháng cự")
	}
	if s.FailedBreakout {
		bear += 0.25
		s.Notes = append(s.Notes, "failed breakout / bull trap")
	}
	if s.Distribution {
		bear += 0.25
		s.Notes = append(s.Notes, "volume mua bị xả gần kháng cự")
	}

	s.BullScore = clamp01(bull)
	s.BearScore = clamp01(bear)
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

func aggregateBias(daily, h4, weekly Signal) Bias {
	bear := weightedBias(daily, h4, weekly, BiasBullTrap) + weightedBias(daily, h4, weekly, BiasDistribution)
	bull := weightedBias(daily, h4, weekly, BiasAccumulation) + weightedBias(daily, h4, weekly, BiasBearTrap)
	if daily.FlowBias == BiasBullTrap || bear >= 0.45 {
		return BiasBullTrap
	}
	if daily.FlowBias == BiasDistribution || bear >= 0.40 {
		return BiasDistribution
	}
	if daily.FlowBias == BiasBearTrap || bull >= 0.45 {
		return BiasBearTrap
	}
	if daily.FlowBias == BiasAccumulation || bull >= 0.35 {
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
