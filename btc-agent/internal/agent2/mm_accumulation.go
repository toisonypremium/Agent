package agent2

import (
	"fmt"
	"math"

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
	Symbol          string      `json:"symbol"`
	Case            MMCase      `json:"case"`
	Score           float64     `json:"score"`
	Pass            bool        `json:"pass"`
	HardBlock       bool        `json:"hard_block"`
	SweepLow        bool        `json:"sweep_low"`
	ReclaimSupport  bool        `json:"reclaim_support"`
	FailedBreakdown bool        `json:"failed_breakdown"`
	Absorption      bool        `json:"absorption"`
	EffortVsResult  bool        `json:"effort_vs_result"`
	SupplyDryUp     bool        `json:"supply_dryup"`
	RetestHold      bool        `json:"retest_hold"`
	Distribution    bool        `json:"distribution"`
	FailedBreakout  bool        `json:"failed_breakout"`
	Support         market.Zone `json:"support"`
	Resistance      market.Zone `json:"resistance"`
	FlowBias        flow.Bias   `json:"flow_bias,omitempty"`
	BullScore       float64     `json:"bull_score,omitempty"`
	BearScore       float64     `json:"bear_score,omitempty"`
	Reasons         []string    `json:"reasons,omitempty"`
	Missing         []string    `json:"missing,omitempty"`
	NextTrigger     string      `json:"next_trigger,omitempty"`
}

func AnalyzeMMAccumulation(symbol string, candles []market.Candle) MMAccumulationSignal {
	s := MMAccumulationSignal{Symbol: symbol, Case: MMCaseNoEdge, NextTrigger: "Chờ sweep low + close reclaim support + retest giữ vùng."}
	if len(candles) < 25 {
		s.Missing = append(s.Missing, "MM footprint chưa đủ dữ liệu")
		s.NextTrigger = "Chờ đủ dữ liệu nến để nhận diện MM footprint."
		return s
	}
	sig := flow.Analyze(candles, "1d", 60)
	s.Support = sig.Support
	s.Resistance = sig.Resistance
	s.FlowBias = sig.FlowBias
	s.BullScore = sig.BullScore
	s.BearScore = sig.BearScore
	s.SweepLow = sig.SweepLow
	s.ReclaimSupport = sig.ReclaimSupport
	s.FailedBreakdown = sig.FailedBreakdown
	s.Absorption = sig.Absorption
	s.Distribution = sig.Distribution
	s.FailedBreakout = sig.FailedBreakout
	s.EffortVsResult = effortVsResult(candles, sig.Support)
	s.SupplyDryUp = supplyDryUp(candles)
	s.RetestHold = retestHold(candles, sig.Support)

	if sig.FlowBias == flow.BiasDistribution || sig.FlowBias == flow.BiasBullTrap || sig.Distribution || sig.FailedBreakout {
		s.Case = MMCaseDistributionTrap
		s.HardBlock = true
		s.Reasons = append(s.Reasons, "MM case DISTRIBUTION_TRAP: phân phối/bull trap, không gom")
		s.NextTrigger = "Chờ hết distribution/bull-trap và reclaim lại support với bull flow."
		s.score()
		return s
	}
	if fallingKnifeBreakdown(candles, sig.Support) {
		s.Case = MMCaseFallingKnife
		s.HardBlock = true
		s.Reasons = append(s.Reasons, "MM case FALLING_KNIFE: breakdown thật, không phải gom")
		s.NextTrigger = "Chờ ngừng lower-low, close reclaim support và volume bán giảm."
		s.score()
		return s
	}

	s.score()
	s.addComponentReasons()
	if s.SweepLow && !s.ReclaimSupport {
		s.Case = MMCaseFailedSweep
		s.Missing = append(s.Missing, "quét đáy nhưng chưa close reclaim support")
		s.NextTrigger = "Chờ close reclaim support sau sweep, tránh bắt dao rơi."
		return s
	}
	if s.ReclaimSupport && (s.Absorption || s.EffortVsResult || s.FailedBreakdown) {
		if s.Score >= 75 && s.RetestHold {
			s.Case = MMCaseArmedProbeCandidate
			s.Pass = true
			s.Reasons = append(s.Reasons, "MM case ARMED_PROBE_CANDIDATE: spring/reclaim + retest đủ mạnh")
			s.NextTrigger = "MM footprint đã tốt; chờ BTC permission, discount, RR, rotation và liquidity cùng pass."
			return s
		}
		s.Case = MMCaseSpringReclaim
		s.Pass = true
		s.Reasons = append(s.Reasons, "MM case SPRING_RECLAIM: sweep/reclaim + absorption")
		s.NextTrigger = "Chờ các gate còn lại pass; không chase ngoài discount zone."
		return s
	}
	if (s.Absorption || s.EffortVsResult) && (!s.ReclaimSupport || !s.RetestHold) {
		s.Case = MMCaseAbsorptionWatch
		if !s.ReclaimSupport {
			s.Missing = append(s.Missing, "có hấp thụ nhưng chưa reclaim support")
		}
		if !s.RetestHold {
			s.Missing = append(s.Missing, "thiếu retest hold sau reclaim")
		}
		s.NextTrigger = "Chờ reclaim/retest giữ support và volume bán cạn."
		return s
	}
	if s.Score >= 60 && s.ReclaimSupport && (s.Absorption || s.EffortVsResult) {
		s.Case = MMCaseSpringReclaim
		s.Pass = true
		s.Reasons = append(s.Reasons, "MM case SPRING_RECLAIM: reclaim + absorption đủ điểm")
		s.NextTrigger = "Chờ các gate còn lại pass; không chase ngoài discount zone."
		return s
	}
	s.Case = MMCaseNoEdge
	s.Missing = append(s.Missing, "chưa thấy sweep/reclaim/absorption đủ rõ")
	s.NextTrigger = "Chờ sweep low + close reclaim support + retest giữ vùng."
	return s
}

func (s *MMAccumulationSignal) score() {
	score := 0.0
	if s.SweepLow && s.ReclaimSupport {
		score += 25
	} else if s.SweepLow {
		score += 10
	}
	if s.Absorption {
		score += 25
	}
	if s.EffortVsResult {
		score += 15
	}
	if s.SupplyDryUp {
		score += 10
	}
	if s.RetestHold {
		score += 10
	}
	if s.FlowBias == flow.BiasAccumulation || s.FlowBias == flow.BiasBearTrap {
		score += 10
	}
	if s.Distribution || s.FailedBreakout {
		score -= 30
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	s.Score = math.Round(score*10) / 10
}

func (s *MMAccumulationSignal) addComponentReasons() {
	if s.SweepLow {
		s.Reasons = append(s.Reasons, "sweep low dưới support")
	}
	if s.ReclaimSupport {
		s.Reasons = append(s.Reasons, "close reclaim support")
	}
	if s.Absorption {
		s.Reasons = append(s.Reasons, "volume bán được hấp thụ gần support")
	}
	if s.EffortVsResult {
		s.Reasons = append(s.Reasons, "effort/result: volume lớn nhưng giá không giảm tương xứng")
	}
	if s.SupplyDryUp {
		s.Reasons = append(s.Reasons, "supply dry-up: volume/range co lại")
	}
	if s.RetestHold {
		s.Reasons = append(s.Reasons, "retest hold trên support")
	}
}

func effortVsResult(c []market.Candle, support market.Zone) bool {
	if len(c) < 22 || !support.Valid() {
		return false
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgVol := avgVolumeMM(c[:len(c)-1], 20)
	avgRange := avgRangeMM(c[:len(c)-1], 20)
	if avgVol <= 0 || avgRange <= 0 || prev.Close <= 0 {
		return false
	}
	volumeHigh := last.Volume >= avgVol*1.5
	result := math.Abs(last.Close-prev.Close) / avgRange
	nearSupport := last.Low <= support.High*1.03 || last.Close <= support.High*1.04
	return volumeHigh && result <= 0.7 && nearSupport && last.Close >= support.Low*0.99
}

func supplyDryUp(c []market.Candle) bool {
	if len(c) < 25 {
		return false
	}
	recentVol := avgVolumeMM(c[len(c)-3:], 3)
	baseVol := avgVolumeMM(c[:len(c)-3], 20)
	recentRange := avgRangeMM(c[len(c)-3:], 3)
	baseRange := avgRangeMM(c[:len(c)-3], 20)
	if baseVol <= 0 || baseRange <= 0 {
		return false
	}
	return recentVol < baseVol*0.85 && recentRange < baseRange*0.85
}

func retestHold(c []market.Candle, support market.Zone) bool {
	if len(c) < 4 || !support.Valid() {
		return false
	}
	start := len(c) - 3
	for _, x := range c[start:] {
		if x.Close < support.Low || x.Low < support.Low*0.97 {
			return false
		}
	}
	return c[len(c)-1].Close >= support.Mid()
}

func fallingKnifeBreakdown(c []market.Candle, support market.Zone) bool {
	if len(c) < 4 || !support.Valid() {
		return false
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgVol := avgVolumeMM(c[:len(c)-1], 20)
	if last.Close >= support.Low || last.Close >= prev.Close {
		return false
	}
	lowerLow := last.Low < prev.Low
	volumeHigh := avgVol > 0 && last.Volume >= avgVol*1.2
	return lowerLow && volumeHigh && !retestHold(c, support)
}

func avgVolumeMM(c []market.Candle, n int) float64 {
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

func avgRangeMM(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	count := 0
	for _, x := range c[start:] {
		r := x.High - x.Low
		if r <= 0 {
			continue
		}
		sum += r
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
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
