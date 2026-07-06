package agent1

import (
	"testing"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestDefaultPermissionPolicyMatchesCurrentThresholds(t *testing.T) {
	p := DefaultPermissionPolicy()
	if p.TrendArmedThreshold != 45 || p.TrendAllowedThreshold != 60 || p.FlowPromoteThreshold != 0.25 || p.PermissionMinRewardRisk != 2.0 {
		t.Fatalf("unexpected defaults: %+v", p)
	}
}

func TestPermissionPolicyNamedProfile(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DecisionProfile = "BALANCED_SAFE"
	p := PermissionPolicyFromConfig(cfg)
	if p.TrendArmedThreshold != 42 || p.TrendAllowedThreshold != 58 || p.FlowPromoteThreshold != 0.22 {
		t.Fatalf("unexpected named profile: %+v", p)
	}
}

func TestPermissionPolicyCustomThresholdsChangePermission(t *testing.T) {
	p := DefaultPermissionPolicy()
	p.TrendArmedThreshold = 35
	got := p.Permission("RANGE", Medium, Low, Low, policySupport(), policyResistance(), 40)
	if got != Armed {
		t.Fatalf("custom armed threshold should allow ARMED, got %s", got)
	}
}

func TestPermissionPolicyHardRiskStillNoTrade(t *testing.T) {
	p := DefaultPermissionPolicy()
	p.TrendArmedThreshold = 1
	p.TrendAllowedThreshold = 2
	got := p.Permission("RANGE", High, Low, Low, policySupport(), policyResistance(), 100)
	if got != NoTrade {
		t.Fatalf("high risk must stay NO_TRADE, got %s", got)
	}
}

func TestPermissionPolicyFlowPromoteThreshold(t *testing.T) {
	p := DefaultPermissionPolicy()
	p.FlowPromoteThreshold = 0.20
	fl := flow.MultiFrame{Bias: flow.BiasAccumulation, Score: 0.21}
	if !p.FlowPromotesToArmed(fl) {
		t.Fatalf("expected flow promote with custom threshold")
	}
}

func policySupport() market.Zone {
	return market.Zone{Name: "support", Low: 100, High: 105}
}

func policyResistance() market.Zone {
	return market.Zone{Name: "resistance", Low: 140, High: 150}
}
