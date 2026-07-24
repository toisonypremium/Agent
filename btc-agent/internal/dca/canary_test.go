package dca

import (
	"btc-agent/internal/agent2"
	"testing"
)

func TestBuildCanaryIntentRequiresExplicitBindingAndCapsNotional(t *testing.T) {
	p := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", Layers: []agent2.Layer{{Index: 1, Price: 2000, Quantity: 1, Notional: 2000}}}}}
	g := GateInput{MarketAllowed: true, LiquidityPass: true, RuntimeHealthy: true, ReconciliationClean: true, ArtifactFresh: true, ThesisRemainingUSDT: 640, ThesisMaxExposureUSDT: 640, EnvelopeUSDT: 1600, GlobalCapPercent: 20}
	if _, err := BuildCanaryIntent(CanaryInput{Plan: p, Gate: g}); err == nil {
		t.Fatal("unbound symbol must not gain thesis")
	}
	out, err := BuildCanaryIntent(CanaryInput{Plan: p, Bindings: []ThesisBinding{{ThesisID: "thesis-eth", Symbol: "ETHUSDT", MaxExposureUSDT: 640, RemainingUSDT: 640}}, Gate: g})
	if err != nil || out.ThesisID != "thesis-eth" || out.Notional != 32 || out.Quantity != .016 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
func TestBuildCanaryIntentFailsWhenPlannerNotActive(t *testing.T) {
	_, err := BuildCanaryIntent(CanaryInput{Plan: agent2.Plan{State: agent2.StateScout}, Gate: GateInput{MarketAllowed: true}})
	if err == nil {
		t.Fatal("expected blocker")
	}
}
