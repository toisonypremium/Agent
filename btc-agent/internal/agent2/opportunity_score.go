package agent2

import (
	"fmt"
	"strings"
)

const (
	OpportunityVerdictStrong  = "STRONG_OPPORTUNITY"
	OpportunityVerdictNormal  = "NORMAL_OPPORTUNITY"
	OpportunityVerdictWatch   = "WATCH_OPPORTUNITY"
	OpportunityVerdictLow     = "LOW_OPPORTUNITY"
	OpportunityVerdictBlocked = "BLOCKED_OPPORTUNITY"
	OpportunityVerdictData    = "BLOCK_DATA"
	OpportunityVerdictRisk    = "BLOCK_RISK"
)

type OpportunityComposite struct {
	Symbol     string             `json:"symbol"`
	Score      float64            `json:"score"`
	BaseScore  float64            `json:"base_score"`
	Penalty    float64            `json:"penalty"`
	Components map[string]float64 `json:"components,omitempty"`
	Verdict    string             `json:"verdict"`
	Reason     string             `json:"reason"`
}

func BuildOpportunityComposite(asset AssetPlan) OpportunityComposite {
	attr := BuildFilterAttribution(asset)
	components := map[string]float64{
		"setup_score":           opportunitySetupComponent(asset),
		"rotation_score":        clamp01(asset.RotationScore),
		"reward_risk_score":     rewardRiskOpportunityComponent(effectiveOpportunityRewardRisk(asset)),
		"discount_score":        discountGapOpportunityComponent(asset.DiscountGapPct),
		"asset_flow_score":      clamp01(asset.AssetFlowScore),
		"mm_accumulation_score": clamp01(asset.MMScore / 100),
		"liquidity_score":       clamp01(asset.LiquidityQuality.Score / 100),
	}
	return OpportunityCompositeFromComponents(asset.Symbol, attr.TopBlockerKey, attr.FailedHard, components)
}

func OpportunityCompositeFromComponents(symbol, topBlockerKey string, failedHard int, components map[string]float64) OpportunityComposite {
	out := OpportunityComposite{Symbol: symbol, Components: map[string]float64{}}
	for key, value := range components {
		out.Components[key] = clamp01(value)
	}
	out.BaseScore = finiteOpportunityScore(100 * (out.Components["setup_score"]*0.30 +
		out.Components["rotation_score"]*0.20 +
		out.Components["reward_risk_score"]*0.15 +
		out.Components["discount_score"]*0.10 +
		out.Components["asset_flow_score"]*0.10 +
		out.Components["mm_accumulation_score"]*0.10 +
		out.Components["liquidity_score"]*0.05))

	key := NormalizeReasonKey(topBlockerKey)
	out.Penalty = opportunityPenalty(key, failedHard, out.Components)
	out.Score = finiteOpportunityScore(out.BaseScore - out.Penalty)
	out.Verdict = opportunityVerdict(out.Score, key, failedHard)
	out.Reason = opportunityReason(out.Verdict, key, failedHard, out.Score)
	return out
}

func opportunitySetupComponent(asset AssetPlan) float64 {
	if asset.SetupScore > 0 {
		return clamp01(asset.SetupScore)
	}
	if asset.State == StateActiveLimit && len(asset.Layers) > 0 {
		return 1
	}
	return 0
}

func effectiveOpportunityRewardRisk(asset AssetPlan) float64 {
	if asset.RewardRisk > 0 {
		return asset.RewardRisk
	}
	best := 0.0
	for _, layer := range asset.Layers {
		if layer.RewardRisk > best {
			best = layer.RewardRisk
		}
	}
	return best
}

func rewardRiskOpportunityComponent(rr float64) float64 {
	if rr <= 0 {
		return 0
	}
	return clamp01(rr / 3.0)
}

func discountGapOpportunityComponent(gap float64) float64 {
	switch {
	case gap <= 0:
		return 1
	case gap <= 0.05:
		return 0.75
	case gap <= 0.12:
		return 0.35
	default:
		return 0.10
	}
}

func opportunityPenalty(key string, failedHard int, components map[string]float64) float64 {
	penalty := 0.0
	if failedHard > 0 {
		penalty += 45
	}
	if key == EntryCheckData {
		penalty += 100
	}
	if key == EntryCheckFallingKnife || key == EntryCheckFOMO {
		penalty += 60
	}
	if components["liquidity_score"] > 0 && components["liquidity_score"] < 0.45 {
		penalty += 15
	}
	return penalty
}

func opportunityVerdict(score float64, key string, failedHard int) string {
	if key == EntryCheckData {
		return OpportunityVerdictData
	}
	if key == EntryCheckFallingKnife || key == EntryCheckFOMO {
		return OpportunityVerdictRisk
	}
	if failedHard > 0 {
		return OpportunityVerdictBlocked
	}
	switch {
	case score >= 80:
		return OpportunityVerdictStrong
	case score >= 65:
		return OpportunityVerdictNormal
	case score >= 45:
		return OpportunityVerdictWatch
	default:
		return OpportunityVerdictLow
	}
}

func opportunityReason(verdict, key string, failedHard int, score float64) string {
	if verdict == OpportunityVerdictData {
		return "thiếu dữ liệu; chỉ nghiên cứu, không phân bổ live"
	}
	if verdict == OpportunityVerdictRisk {
		return "rủi ro kỹ thuật cao; không nới gate"
	}
	if failedHard > 0 {
		return "có hard blocker; cơ hội bị chặn nghiên cứu"
	}
	if key != "" && key != "UNKNOWN" {
		return fmt.Sprintf("score %.1f; blocker chính %s; research-only", score, key)
	}
	return fmt.Sprintf("score %.1f; research-only", score)
}

func finiteOpportunityScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func OpportunityCompositeBlocked(verdict string) bool {
	verdict = strings.ToUpper(strings.TrimSpace(verdict))
	return verdict == OpportunityVerdictBlocked || verdict == OpportunityVerdictData || verdict == OpportunityVerdictRisk
}
