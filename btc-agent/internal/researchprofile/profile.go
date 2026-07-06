package researchprofile

import (
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/flow"
)

type Profile struct {
	Name                  string  `json:"name"`
	TrendArmedThreshold   float64 `json:"trend_armed_threshold"`
	TrendAllowedThreshold float64 `json:"trend_allowed_threshold"`
	FlowPromoteThreshold  float64 `json:"flow_promote_threshold"`
	MinRewardRisk         float64 `json:"min_reward_risk"`
	ResearchNote          string  `json:"research_note"`
}

func Profiles() []Profile {
	return []Profile{
		{Name: "STRICT_CURRENT", TrendArmedThreshold: 45, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.25, MinRewardRisk: 2.0, ResearchNote: "current production thresholds"},
		{Name: "BALANCED_SAFE", TrendArmedThreshold: 42, TrendAllowedThreshold: 58, FlowPromoteThreshold: 0.22, MinRewardRisk: 2.0, ResearchNote: "research-only mild threshold relaxation"},
		{Name: "ARMED_PROBE_LIGHT", TrendArmedThreshold: 40, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.20, MinRewardRisk: 2.0, ResearchNote: "research-only probe candidate density"},
		{Name: "FLOW_RELAXED", TrendArmedThreshold: 45, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.15, MinRewardRisk: 2.0, ResearchNote: "research-only flow promotion sensitivity"},
		{Name: "RR_RELAXED_SMALL_PROBE", TrendArmedThreshold: 42, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.22, MinRewardRisk: 1.5, ResearchNote: "research-only lower RR for small probe review"},
	}
}

func ProfileByName(name string) (Profile, bool) {
	for _, p := range Profiles() {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Profile{}, false
}

func EvaluatePermission(a agent1.MarketAnalysis, p Profile) agent1.Permission {
	if a.MarketRegime == "PANIC_SELLING" || a.RiskLevel == agent1.High || a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High || !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return agent1.NoTrade
	}
	if permissionRRProxy(a) < p.MinRewardRisk {
		return agent1.Watch
	}
	allowedRegime := a.MarketRegime == "ACCUMULATION" || a.MarketRegime == "WEAK_UPTREND" || a.MarketRegime == "RANGE"
	if a.TrendScore >= p.TrendAllowedThreshold && allowedRegime {
		return agent1.Allowed
	}
	if a.TrendScore >= p.TrendArmedThreshold {
		return agent1.Armed
	}
	flowOK := (a.Flow.Bias == flow.BiasAccumulation || a.Flow.Bias == flow.BiasBearTrap) && a.Flow.Score >= p.FlowPromoteThreshold
	if flowOK {
		return agent1.Armed
	}
	return agent1.Watch
}

func permissionRRProxy(a agent1.MarketAnalysis) float64 {
	if !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return 0
	}
	entry := a.PrimarySupportZone.High
	invalidation := a.PrimarySupportZone.Low * 0.985
	risk := entry - invalidation
	if risk <= 0 {
		return 0
	}
	return (a.ResistanceZone.High - entry) / risk
}
