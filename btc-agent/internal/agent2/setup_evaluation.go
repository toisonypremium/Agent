package agent2

import (
	"fmt"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/indicators"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

type SetupGateSeverity string

const (
	SetupGateHard SetupGateSeverity = "HARD"
	SetupGateSoft SetupGateSeverity = "SOFT"
)

type SetupGateResult struct {
	Name     string            `json:"name"`
	Pass     bool              `json:"pass"`
	Severity SetupGateSeverity `json:"severity"`
	Score    float64           `json:"score"`
	Reason   string            `json:"reason"`
	Next     string            `json:"next"`
}

type SetupEvaluation struct {
	Symbol         string            `json:"symbol"`
	Score          float64           `json:"score"`
	HardBlockers   []string          `json:"hard_blockers,omitempty"`
	SoftBlockers   []string          `json:"soft_blockers,omitempty"`
	Gates          []SetupGateResult `json:"gates"`
	Actionable     bool              `json:"actionable"`
	NearActionable bool              `json:"near_actionable"`
	NextTrigger    string            `json:"next_trigger"`
}

func evaluateAssetSetup(cfg config.Config, sym string, c []market.Candle, benchmark []market.Candle, rotation AssetRotationScore, useAssetFlowEntry bool) (AssetPlan, SetupEvaluation) {
	ap := AssetPlan{Symbol: sym, State: StateWatch, Reason: "chưa đủ dữ liệu hoặc chưa vào discount zone", NextTrigger: "Chờ đủ dữ liệu và giá về discount zone."}
	eval := SetupEvaluation{Symbol: sym}
	add := func(name string, severity SetupGateSeverity, pass bool, score float64, reason, next string) {
		gate := SetupGateResult{Name: name, Severity: severity, Pass: pass, Score: clamp01(score), Reason: reason, Next: next}
		eval.Gates = append(eval.Gates, gate)
		if pass {
			return
		}
		if severity == SetupGateHard {
			eval.HardBlockers = append(eval.HardBlockers, reason)
		} else {
			eval.SoftBlockers = append(eval.SoftBlockers, reason)
		}
	}
	if len(c) < 60 {
		reason := "chưa đủ dữ liệu 1D"
		add("DATA", SetupGateHard, false, 0, reason, "Chờ đủ dữ liệu nến 1D trước khi đánh giá candidate.")
		finalizeSetupEvaluation(&eval, cfg)
		ap.HardBlockers = append(ap.HardBlockers, eval.HardBlockers...)
		ap.SetupScore = eval.Score
		ap.SetupGates = eval.Gates
		ap.Reason = reason
		ap.NextTrigger = eval.NextTrigger
		return ap, eval
	}

	price := market.LastClose(c)
	support, resistance := actionSupportResistanceZones(c)
	closes := make([]float64, len(c))
	for i, candle := range c {
		closes[i] = candle.Close
	}
	ema20 := indicators.Last(indicators.EMA(closes, 20))
	rsi := indicators.Last(indicators.RSI(closes, 14))

	ap.DiscountZone = support
	ap.ZoneWidthPct = zoneWidthPct(support)
	ap.DiscountGapPct = discountGapPct(price, support)
	ap.ZoneQuality = zoneQuality(support)
	mm := AnalyzeMMAccumulation(sym, c)
	ap.MMCase = mm.Case
	ap.MMScore = mm.Score
	ap.MMReasons = mm.Reasons
	ap.MMMissing = mm.Missing
	ap.AssetFlowBias = mm.FlowBias
	ap.AssetFlowScore = mm.BullScore
	ap.LiquidityQuality = liquidity.EvaluateCandleProxy(cfg, sym, c, desiredLiquidityNotional(cfg, sym))

	zonePass := ap.ZoneQuality == "ZONE_OK"
	add("ZONE_QUALITY", SetupGateSoft, zonePass, boolScore(zonePass, 1, 0.25), zoneQualityReason(ap.ZoneQuality, ap.ZoneWidthPct), "Chờ vùng support/discount hẹp và rõ hơn để tính entry/RR.")

	falling := FallingKnife(c)
	add(EntryCheckFallingKnife, SetupGateHard, !falling, boolScore(!falling, 1, 0), "falling knife risk", "Chờ cấu trúc ngừng lower-low và reclaim support rõ.")
	fomo := FOMO(c, ema20, rsi, resistance)
	add(EntryCheckFOMO, SetupGateHard, !fomo, boolScore(!fomo, 1, 0), "FOMO risk", "Không đuổi giá; chờ pullback về value/support.")

	if enabled, lookback, minRelative, minMomentum := relativeStrengthParams(cfg); enabled && len(benchmark) > 0 {
		rs := RelativeStrength(c, benchmark, lookback, minRelative, minMomentum)
		add(EntryCheckRelativeStrength, SetupGateHard, rs.Pass, relativeComponent(rs.RelativeReturn), rs.Reason, "Chờ asset ngừng underperform BTC trong lookback.")
	}
	if enabled, minScore, maxRank := rotationParams(cfg); enabled && rotation.Symbol != "" {
		ap.RotationRank = rotation.Rank
		ap.RotationScore = rotation.Score
		scorePass := rotation.Eligible && rotation.Score >= minScore
		add(EntryCheckRotationScore, SetupGateSoft, scorePass, rotation.Score, fmt.Sprintf("rotation score filter chặn asset: rank=%d score=%.2f reason=%s", rotation.Rank, rotation.Score, rotation.Reason), "Chờ rotation score tăng hoặc rank vào top được phép.")
		if maxRank > 0 {
			rankPass := rotation.Rank <= maxRank
			add(EntryCheckRotationRank, SetupGateSoft, rankPass, boolScore(rankPass, 1, 0.4), fmt.Sprintf("rotation score filter chặn asset: rank=%d ngoài top %d score=%.2f reason=%s", rotation.Rank, maxRank, rotation.Score, rotation.Reason), "Chờ rotation rank vào top được phép.")
		}
	}

	if mm.HardBlock {
		add(EntryCheckMMAccumulation, SetupGateHard, false, 0, mmReason(mm), mm.NextTrigger)
	} else if useAssetFlowEntry {
		add(EntryCheckMMAccumulation, SetupGateSoft, mm.Pass, clamp01(mm.Score/100), mmReason(mm), mm.NextTrigger)
	}
	if enabled, minBull, allowNeutral := assetFlowEntryParams(cfg); enabled && useAssetFlowEntry {
		entry := AssetFlowEntryFromMM(mm, minBull, allowNeutral)
		ap.AssetFlowBias = entry.Bias
		ap.AssetFlowScore = entry.BullScore
		severity := SetupGateSoft
		if entry.HardBlock {
			severity = SetupGateHard
		}
		reason := entry.Reason
		if !entry.Pass && !entry.HardBlock && reason != "" && !containsAny(strings.ToLower(reason), "asset flow entry") {
			reason = "asset flow entry chưa xác nhận: " + reason
		}
		add(EntryCheckAssetFlowEntry, severity, entry.Pass, entryScore(entry), reason, firstNonEmptyMM(entry.NextTrigger, "Chờ sweep low + reclaim support hoặc absorption volume gần support."))
	}
	if cfg.Live.LiquidityGateEnabled && ap.LiquidityQuality.Enabled {
		add(EntryCheckLiquidityQuality, SetupGateSoft, ap.LiquidityQuality.Pass, clamp01(ap.LiquidityQuality.Score/100), "liquidity gate blocked: "+liquidity.FirstReason(ap.LiquidityQuality.Reasons), "Chờ spread/depth/volume đủ dày trước khi tạo live layer.")
	}
	discountPass := support.Valid() && price <= support.High*(1+discountZonePremiumPct(cfg))
	add(EntryCheckDiscountZone, SetupGateSoft, discountPass, discountComponent(price, support), fmt.Sprintf("giá chưa vào discount zone: cao hơn support %.2f%%", ap.DiscountGapPct*100), "Chờ giá về support/discount zone mà không tạo falling knife.")

	invalidation := support.Low * 0.985
	rr := RewardRiskBreakdown(RewardRiskInput{Entry: price, Invalidation: invalidation, Target: resistance.High})
	ap.RewardRiskDetail = rr
	ap.DiscountZone = support
	ap.Invalidation = invalidation
	if rr.Valid {
		ap.RewardRisk = rr.Ratio
		add(EntryCheckRewardRisk, SetupGateSoft, rr.Ratio >= cfg.Risk.MinRewardRisk, clamp01(rr.Ratio/cfg.Risk.MinRewardRisk), fmt.Sprintf("reward/risk %.2f thấp hơn %.2f; risk %.4f reward %.4f", rr.Ratio, cfg.Risk.MinRewardRisk, rr.Risk, rr.Reward), "Chờ entry sâu hơn hoặc resistance mở rộng để RR đạt ngưỡng.")
	} else {
		add(EntryCheckRewardRisk, SetupGateSoft, false, 0, "reward/risk không hợp lệ: "+rr.Reason, "Chờ entry sâu hơn hoặc target rõ hơn để tính RR.")
	}

	finalizeSetupEvaluation(&eval, cfg)
	ap.SetupScore = eval.Score
	ap.SetupGates = eval.Gates
	ap.HardBlockers = append(ap.HardBlockers, eval.HardBlockers...)
	ap.SoftBlockers = append(ap.SoftBlockers, eval.SoftBlockers...)
	ap.NextTrigger = eval.NextTrigger
	return ap, eval
}

func finalizeSetupEvaluation(eval *SetupEvaluation, cfg config.Config) {
	passed := 0
	total := 0
	score := 0.0
	for _, gate := range eval.Gates {
		total++
		score += gate.Score
		if gate.Pass {
			passed++
		}
		if !gate.Pass && eval.NextTrigger == "" {
			eval.NextTrigger = gate.Next
		}
	}
	if total > 0 {
		eval.Score = clamp01(score / float64(total))
	}
	if eval.NextTrigger == "" {
		eval.NextTrigger = "Đã đủ điều kiện theo deterministic setup evaluation."
	}
	eval.Actionable = len(eval.HardBlockers) == 0 && len(eval.SoftBlockers) == 0 && passed == total && total > 0
	minNear := cfg.Risk.MinWatchReadinessForProbe
	if minNear <= 0 {
		minNear = 0.70
	}
	eval.NearActionable = len(eval.HardBlockers) == 0 && eval.Score >= minNear
	eval.HardBlockers = uniqueStrings(eval.HardBlockers)
	eval.SoftBlockers = uniqueStrings(eval.SoftBlockers)
}

func boolScore(pass bool, passScore, failScore float64) float64 {
	if pass {
		return passScore
	}
	return failScore
}

func entryScore(entry AssetFlowEntrySignal) float64 {
	if entry.Pass {
		return 1
	}
	if entry.HardBlock {
		return 0
	}
	return 0.5
}
