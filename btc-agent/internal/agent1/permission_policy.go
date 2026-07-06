package agent1

import (
	"fmt"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	DefaultDecisionProfile            = "STRICT_CURRENT"
	DefaultBTCTrendArmedThreshold     = 45.0
	DefaultBTCTrendAllowedThreshold   = 60.0
	DefaultBTCFlowPromoteThreshold    = 0.25
	DefaultBTCPermissionMinRewardRisk = 2.0
)

type PermissionPolicy struct {
	DecisionProfile         string  `json:"decision_profile"`
	TrendArmedThreshold     float64 `json:"trend_armed_threshold"`
	TrendAllowedThreshold   float64 `json:"trend_allowed_threshold"`
	FlowPromoteThreshold    float64 `json:"flow_promote_threshold"`
	PermissionMinRewardRisk float64 `json:"permission_min_reward_risk"`
}

func DefaultPermissionPolicy() PermissionPolicy {
	return PermissionPolicy{
		DecisionProfile:         DefaultDecisionProfile,
		TrendArmedThreshold:     DefaultBTCTrendArmedThreshold,
		TrendAllowedThreshold:   DefaultBTCTrendAllowedThreshold,
		FlowPromoteThreshold:    DefaultBTCFlowPromoteThreshold,
		PermissionMinRewardRisk: DefaultBTCPermissionMinRewardRisk,
	}
}

func PermissionPolicyFromConfig(cfg config.Config) PermissionPolicy {
	p := DefaultPermissionPolicy()
	if strings.TrimSpace(cfg.Risk.DecisionProfile) != "" {
		p.DecisionProfile = strings.ToUpper(strings.TrimSpace(cfg.Risk.DecisionProfile))
		p = applyNamedPermissionProfile(p)
	}
	if cfg.Risk.BTCTrendArmedThreshold > 0 {
		p.TrendArmedThreshold = cfg.Risk.BTCTrendArmedThreshold
	}
	if cfg.Risk.BTCTrendAllowedThreshold > 0 {
		p.TrendAllowedThreshold = cfg.Risk.BTCTrendAllowedThreshold
	}
	if cfg.Risk.BTCFlowPromoteThreshold > 0 {
		p.FlowPromoteThreshold = cfg.Risk.BTCFlowPromoteThreshold
	}
	if cfg.Risk.BTCPermissionMinRewardRisk > 0 {
		p.PermissionMinRewardRisk = cfg.Risk.BTCPermissionMinRewardRisk
	}
	return p
}

func applyNamedPermissionProfile(p PermissionPolicy) PermissionPolicy {
	switch p.DecisionProfile {
	case "BALANCED_SAFE":
		p.TrendArmedThreshold = 42
		p.TrendAllowedThreshold = 58
		p.FlowPromoteThreshold = 0.22
		p.PermissionMinRewardRisk = 2.0
	case "ARMED_PROBE_LIGHT":
		p.TrendArmedThreshold = 40
		p.TrendAllowedThreshold = 60
		p.FlowPromoteThreshold = 0.20
		p.PermissionMinRewardRisk = 2.0
	case "FLOW_RELAXED":
		p.TrendArmedThreshold = 45
		p.TrendAllowedThreshold = 60
		p.FlowPromoteThreshold = 0.15
		p.PermissionMinRewardRisk = 2.0
	case "RR_RELAXED_SMALL_PROBE":
		p.TrendArmedThreshold = 42
		p.TrendAllowedThreshold = 60
		p.FlowPromoteThreshold = 0.22
		p.PermissionMinRewardRisk = 1.5
	}
	return p
}

func (p PermissionPolicy) Permission(regime string, risk Risk, falling Risk, fomo Risk, support, resistance market.Zone, trend float64) Permission {
	if regime == "PANIC_SELLING" || falling == High || fomo == High || risk == High {
		return NoTrade
	}
	if !support.Valid() || !resistance.Valid() {
		return NoTrade
	}
	if permissionRewardRisk(support, resistance) < p.PermissionMinRewardRisk {
		return Watch
	}
	if trend >= p.TrendAllowedThreshold && (regime == "ACCUMULATION" || regime == "WEAK_UPTREND" || regime == "RANGE") {
		return Allowed
	}
	if trend >= p.TrendArmedThreshold {
		return Armed
	}
	return Watch
}

func (p PermissionPolicy) PermissionReason(regime string, risk Risk, falling Risk, fomo Risk, support, resistance market.Zone, trend float64, perm Permission) string {
	blockers := riskBlockers(regime, risk, falling, fomo, support, resistance)
	if len(blockers) > 0 {
		return "blocked: " + strings.Join(blockers, "; ")
	}
	rr := permissionRewardRisk(support, resistance)
	if rr < p.PermissionMinRewardRisk {
		return fmt.Sprintf("reward/risk proxy %.2f dưới %.2f", rr, p.PermissionMinRewardRisk)
	}
	switch perm {
	case Allowed:
		return fmt.Sprintf("trend %.1f và regime %s đủ cho ALLOWED", trend, regime)
	case Armed:
		return fmt.Sprintf("trend %.1f chỉ đủ ARMED", trend)
	case Watch:
		return fmt.Sprintf("trend %.1f chưa đủ ARMED", trend)
	default:
		return string(perm)
	}
}

func (p PermissionPolicy) FlowPromotesToArmed(f flow.MultiFrame) bool {
	return (f.Bias == flow.BiasAccumulation || f.Bias == flow.BiasBearTrap) && f.Score >= p.FlowPromoteThreshold
}
