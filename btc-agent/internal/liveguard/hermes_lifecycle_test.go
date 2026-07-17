package liveguard

import (
	"btc-agent/internal/agent2"
	"btc-agent/internal/flow"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
	"strings"
	"testing"
	"time"
)

func lifecycleAsset() agent2.AssetPlan {
	return agent2.AssetPlan{Symbol: "ETHUSDT", State: agent2.StateArmed, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 85, RewardRisk: 3.5, MMScore: 65, AssetFlowScore: .3, AssetFlowBias: flow.BiasAccumulation, SetupScore: .7, LiquidityQuality: liquidity.Quality{Enabled: true, Pass: true}}
}
func lifecycleAction(i hermesoperator.Intent) hermesoperator.Action {
	return hermesoperator.Action{Symbol: "ETHUSDT", Intent: i, EntryPrice: 95, Invalidation: 85, Target: 120, Confidence: .8}
}
func TestHermesLifecycleProbeAllowedWithEvidence(t *testing.T) {
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentProbeLimit), Asset: lifecycleAsset(), AssetCap: 100})
	if !r.Allowed {
		t.Fatalf("probe blocked: %+v", r)
	}
}
func TestHermesLifecycleCannotSkipProbe(t *testing.T) {
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentOpenLimit), Asset: lifecycleAsset(), AssetCap: 100})
	if r.Allowed || !strings.Contains(LifecycleReasonText(r.Reasons), "filled probe") {
		t.Fatalf("open skip allowed: %+v", r)
	}
}
func TestHermesLifecycleOpenAfterProbe(t *testing.T) {
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentOpenLimit), Asset: lifecycleAsset(), AssetCap: 100, ExistingNotional: 20})
	if !r.Allowed {
		t.Fatalf("open after probe blocked: %+v", r)
	}
}
func TestHermesLifecyclePendingOrderDefersStage(t *testing.T) {
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentOpenLimit), Asset: lifecycleAsset(), AssetCap: 100, ExistingNotional: 20, HasOpenBuy: true})
	if r.Allowed {
		t.Fatalf("pending order stage allowed: %+v", r)
	}
}
func TestHermesLifecycleScaleRequiresStrongConfirmation(t *testing.T) {
	a := lifecycleAsset()
	a.State = agent2.StateScout
	a.MMScore = 20
	a.AssetFlowScore = 0
	a.SetupScore = .2
	a.RewardRisk = 1.8
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentScaleLimit), Asset: a, AssetCap: 100, ExistingNotional: 30})
	if r.Allowed || r.Confirmations >= r.Required {
		t.Fatalf("weak scale allowed: %+v", r)
	}
}

func TestHermesLifecycleReentryCooldown(t *testing.T) {
	now := time.Date(2026, 7, 17, 2, 0, 0, 0, time.UTC)
	r := EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentProbeLimit), Asset: lifecycleAsset(), AssetCap: 100, Now: now, LastExitAt: now.Add(-time.Hour), CooldownAfterExit: 4 * time.Hour})
	if r.Allowed || !strings.Contains(LifecycleReasonText(r.Reasons), "cooldown") {
		t.Fatalf("cooldown did not block: %+v", r)
	}
	r = EvaluateHermesLifecycle(HermesLifecycleContext{Action: lifecycleAction(hermesoperator.IntentProbeLimit), Asset: lifecycleAsset(), AssetCap: 100, Now: now, LastExitAt: now.Add(-5 * time.Hour), CooldownAfterExit: 4 * time.Hour})
	if !r.Allowed {
		t.Fatalf("expired cooldown blocked: %+v", r)
	}
}

func TestFallingKnifeExceptionalRRLifecycleIsProbeOnly(t *testing.T) {
	asset := agent2.AssetPlan{Symbol: "RENDERUSDT", State: agent2.StateScout, DiscountZone: market.Zone{Low: 1.4, High: 1.6}, Invalidation: 1.3, RewardRisk: 16.8, SetupScore: .74, MMScore: 25, AssetFlowScore: 0}
	base := HermesLifecycleContext{Asset: asset, AssetCap: 1000, Now: time.Now(), FallingKnifeHigh: true, ExceptionalRRThreshold: 6}
	probe := base
	probe.Action = hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentProbeLimit, EntryPrice: 1.48, Invalidation: 1.42, Target: 2.43}
	if got := EvaluateHermesLifecycle(probe); !got.Allowed {
		t.Fatalf("exceptional RR probe should pass deterministic lifecycle: %+v", got)
	}
	open := base
	open.ExistingNotional = 50
	open.Action = hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentOpenLimit, EntryPrice: 1.48, Invalidation: 1.42, Target: 2.43}
	if got := EvaluateHermesLifecycle(open); got.Allowed {
		t.Fatalf("falling knife must block OPEN: %+v", got)
	}
	scale := base
	scale.ExistingNotional = 200
	scale.Action = hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentScaleLimit, EntryPrice: 1.48, Invalidation: 1.42, Target: 2.43}
	if got := EvaluateHermesLifecycle(scale); got.Allowed {
		t.Fatalf("falling knife must block SCALE: %+v", got)
	}
}
func TestFallingKnifeProbeRejectsNonExceptionalRR(t *testing.T) {
	asset := agent2.AssetPlan{Symbol: "ETHUSDT", State: agent2.StateScout, DiscountZone: market.Zone{Low: 90, High: 110}, RewardRisk: 3, SetupScore: .8}
	got := EvaluateHermesLifecycle(HermesLifecycleContext{Action: hermesoperator.Action{Symbol: "ETHUSDT", Intent: hermesoperator.IntentProbeLimit, EntryPrice: 100, Invalidation: 90, Target: 130}, Asset: asset, AssetCap: 1000, FallingKnifeHigh: true, ExceptionalRRThreshold: 6})
	if got.Allowed {
		t.Fatalf("non-exceptional RR probe must stay blocked: %+v", got)
	}
}
