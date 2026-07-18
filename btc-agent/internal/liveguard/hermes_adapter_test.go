package liveguard

import (
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/market"
)

func TestEvaluateHermesActionsAllowsRiskReducingActionDuringHalt(t *testing.T) {
	got := EvaluateHermesActions([]hermesoperator.Action{{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentCancel}}, HermesSafetyContext{OperatorHalted: true})
	if len(got) != 1 || !got[0].Allowed {
		t.Fatalf("cancel should remain allowed: %+v", got)
	}
}

func TestEvaluateHermesActionsBlocksExposureOnHardSafety(t *testing.T) {
	action := hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentProbeLimit, RequestedNotionalUSDT: 5}
	got := EvaluateHermesActions([]hermesoperator.Action{action}, HermesSafetyContext{DataHealthy: true, ReconcileClean: true, OKXReady: true, PortfolioNotionalRemaining: 100, AssetNotionalRemaining: map[string]float64{"RENDERUSDT": 20}, PanicSelling: true})
	if len(got) != 1 || got[0].Allowed {
		t.Fatalf("panic should block exposure: %+v", got)
	}
}

func TestEvaluateHermesActionsCapsNotional(t *testing.T) {
	action := hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentProbeLimit, RequestedNotionalUSDT: 50}
	got := EvaluateHermesActions([]hermesoperator.Action{action}, HermesSafetyContext{DataHealthy: true, ReconcileClean: true, OKXReady: true, PortfolioNotionalRemaining: 10, AssetNotionalRemaining: map[string]float64{"RENDERUSDT": 20}})
	if len(got) != 1 || !got[0].Allowed || got[0].NotionalUSDT != 10 {
		t.Fatalf("expected capped action: %+v", got)
	}
}

func TestBuildHermesShadowDesiredOrdersAllowsScoutProbeInEnvelope(t *testing.T) {
	cfg := config.Config{}
	cfg.Live.MaxOrderNotionalUSDT = 100
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 100
	cfg.Risk.DiscountZonePremiumPct = 0
	plan := agent2.Plan{State: agent2.StateScout, Assets: []agent2.AssetPlan{{Symbol: "RENDERUSDT", State: agent2.StateScout, DiscountZone: market.Zone{Low: 1.4, High: 1.6}, Invalidation: 1.3, RewardRisk: 8}}}
	decisions := []HermesActionDecision{{Allowed: true, NotionalUSDT: 5, Action: hermesoperator.Action{ThesisID: "thesis-hermes", Symbol: "RENDERUSDT", Intent: hermesoperator.IntentProbeLimit, EntryPrice: 1.5, Target: 2.4, ReasonCodes: []string{"EXCEPTIONAL_RR"}}}}
	desired, blocked := BuildHermesShadowDesiredOrders(cfg, plan, decisions, nil)
	if len(blocked) != 0 || len(desired) != 1 {
		t.Fatalf("desired=%+v blocked=%+v", desired, blocked)
	}
	if desired[0].ThesisID != "thesis-hermes" || desired[0].Source != "HERMES_SHADOW" || desired[0].Notional > 5.000001 || !desired[0].PostOnly {
		t.Fatalf("unsafe shadow desired: %+v", desired[0])
	}
}

func TestBuildHermesShadowDesiredOrdersBlocksOutsidePriceEnvelope(t *testing.T) {
	cfg := config.Config{}
	cfg.Live.MaxOrderNotionalUSDT = 100
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 100
	plan := agent2.Plan{State: agent2.StateScout, Assets: []agent2.AssetPlan{{Symbol: "RENDERUSDT", State: agent2.StateScout, DiscountZone: market.Zone{Low: 1.4, High: 1.6}}}}
	decisions := []HermesActionDecision{{Allowed: true, NotionalUSDT: 5, Action: hermesoperator.Action{Symbol: "RENDERUSDT", Intent: hermesoperator.IntentProbeLimit, EntryPrice: 2}}}
	desired, blocked := BuildHermesShadowDesiredOrders(cfg, plan, decisions, nil)
	if len(desired) != 0 || len(blocked) != 1 {
		t.Fatalf("desired=%+v blocked=%+v", desired, blocked)
	}
}

func TestEvaluateHermesActionsBlocksReduceWithoutCleanReconcileOrOKX(t *testing.T) {
	action := hermesoperator.Action{Symbol: "BTCUSDT", Intent: hermesoperator.IntentReduce, RequestedNotionalUSDT: 10, EntryPrice: 50000}
	got := EvaluateHermesActions([]hermesoperator.Action{action}, HermesSafetyContext{OperatorHalted: true})
	if len(got) != 1 || got[0].Allowed || len(got[0].Reasons) != 2 {
		t.Fatalf("unsafe reduce allowed: %+v", got)
	}
	good := EvaluateHermesActions([]hermesoperator.Action{action}, HermesSafetyContext{OperatorHalted: true, ReconcileClean: true, OKXReady: true})
	if len(good) != 1 || !good[0].Allowed {
		t.Fatalf("safe reduce blocked during halt: %+v", good)
	}
}
