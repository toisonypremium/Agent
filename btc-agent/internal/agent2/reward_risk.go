package agent2

import "btc-agent/internal/market"

type RewardRiskInput struct {
	Entry        float64
	Invalidation float64
	Target       float64
}

type RewardRiskResult struct {
	Entry        float64 `json:"entry"`
	Invalidation float64 `json:"invalidation"`
	Target       float64 `json:"target"`
	Risk         float64 `json:"risk"`
	Reward       float64 `json:"reward"`
	Ratio        float64 `json:"ratio"`
	Valid        bool    `json:"valid"`
	Reason       string  `json:"reason,omitempty"`
}

func RewardRiskBreakdown(in RewardRiskInput) RewardRiskResult {
	out := RewardRiskResult{Entry: in.Entry, Invalidation: in.Invalidation, Target: in.Target}
	if in.Entry <= 0 || in.Invalidation <= 0 || in.Target <= 0 {
		out.Reason = "missing entry/invalidation/target"
		return out
	}
	out.Risk = in.Entry - in.Invalidation
	out.Reward = in.Target - in.Entry
	if out.Risk <= 0 {
		out.Reason = "entry not above invalidation"
		return out
	}
	if out.Reward <= 0 {
		out.Reason = "target not above entry"
		return out
	}
	out.Ratio = out.Reward / out.Risk
	out.Valid = true
	return out
}

func RewardRiskFromZones(entry float64, support, resistance market.Zone) RewardRiskResult {
	if support.Valid() && resistance.Valid() {
		return RewardRiskBreakdown(RewardRiskInput{Entry: entry, Invalidation: support.Low * 0.985, Target: resistance.High})
	}
	return RewardRiskBreakdown(RewardRiskInput{Entry: entry})
}
