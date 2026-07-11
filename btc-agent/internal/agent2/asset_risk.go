package agent2

import "btc-agent/internal/market"

type AssetRiskSignal struct {
	FallingKnife ReasonSeverity   `json:"falling_knife"`
	FOMO         ReasonSeverity   `json:"fomo"`
	Reasons      []DecisionReason `json:"reasons,omitempty"`
}

func ClassifyAssetRisk(c []market.Candle, ema20, rsi float64, resistance market.Zone) AssetRiskSignal {
	s := AssetRiskSignal{FallingKnife: ReasonInfo, FOMO: ReasonInfo}
	if len(c) < 5 {
		s.FallingKnife = ReasonHardBlock
		s.Reasons = AddReason(s.Reasons, NewDecisionReason(ReasonDataWait, ReasonHardBlock, ReasonScopeData, "asset risk chưa đủ dữ liệu"))
		return s
	}
	lowerLows := 0
	for i := len(c) - 4; i < len(c); i++ {
		if c[i].Low < c[i-1].Low {
			lowerLows++
		}
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgRange := avgRangeAssetRisk(c[:len(c)-1], 20)
	avgVol := avgVolumeAssetRisk(c[:len(c)-1], 20)
	breakPrevLow := last.Close < prev.Low
	volumeSpike := last.Volume > prev.Volume*1.5 || (avgVol > 0 && last.Volume > avgVol*1.5)
	atrExpansion := avgRange > 0 && (last.High-last.Low) > avgRange*1.5
	if breakPrevLow && (volumeSpike || atrExpansion) {
		s.FallingKnife = ReasonHardBlock
		s.Reasons = AddReason(s.Reasons, NewDecisionReason(ReasonFallingKnife, ReasonHardBlock, ReasonScopeRisk, "falling knife hard: close dưới previous low kèm volume/range spike"))
	} else if lowerLows >= 3 {
		s.FallingKnife = ReasonSoftWait
		s.Reasons = AddReason(s.Reasons, NewDecisionReason(ReasonFallingKnife, ReasonSoftWait, ReasonScopeRisk, "falling knife soft: nhiều lower-low nhưng chưa breakdown volume xác nhận"))
	}

	green := 0
	for i := len(c) - 4; i < len(c); i++ {
		if c[i].Close > c[i].Open {
			green++
		}
	}
	price := last.Close
	nearResistance := resistance.Valid() && price > resistance.Low*0.98
	extendedEMA := ema20 > 0 && price > ema20*1.12
	if (nearResistance && rsi > 70) || extendedEMA && rsi > 72 {
		s.FOMO = ReasonHardBlock
		s.Reasons = AddReason(s.Reasons, NewDecisionReason(ReasonFOMO, ReasonHardBlock, ReasonScopeRisk, "FOMO hard: RSI cao gần resistance hoặc extension quá xa EMA20"))
	} else if green >= 4 || (ema20 > 0 && price > ema20*1.08) || rsi > 68 || nearResistance {
		s.FOMO = ReasonSoftWait
		s.Reasons = AddReason(s.Reasons, NewDecisionReason(ReasonFOMO, ReasonSoftWait, ReasonScopeRisk, "FOMO soft: nhịp hồi nóng, chờ pullback/retest"))
	}
	return s
}

func avgVolumeAssetRisk(c []market.Candle, n int) float64 {
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

func avgRangeAssetRisk(c []market.Candle, n int) float64 {
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
