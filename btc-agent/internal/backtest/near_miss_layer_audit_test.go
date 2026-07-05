package backtest

import (
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func TestNearMissLayerCandidateSkipsInvalidAndDanger(t *testing.T) {
	base := agent2.WatchCandidate{Symbol: "ETHUSDT", Price: 100, Support: market.Zone{Low: 90, High: 100}, EntryChecklist: []agent2.EntryChecklistItem{{Name: agent2.EntryCheckBTCPermission, Severity: agent2.EntryCheckHard, Pass: false}}}
	if !nearMissLayerCandidateOK(base) {
		t.Fatalf("BTC-not-allowed near-miss should be eligible for research")
	}
	base.Actionable = true
	if nearMissLayerCandidateOK(base) {
		t.Fatalf("actionable candidate should be skipped")
	}
	base.Actionable = false
	base.EntryChecklist = []agent2.EntryChecklistItem{{Name: agent2.EntryCheckFallingKnife, Severity: agent2.EntryCheckHard, Pass: false}}
	if nearMissLayerCandidateOK(base) {
		t.Fatalf("falling knife hard fail should be skipped")
	}
	base.EntryChecklist = []agent2.EntryChecklistItem{{Name: agent2.EntryCheckAssetFlowEntry, Severity: agent2.EntryCheckHard, Pass: false}}
	if nearMissLayerCandidateOK(base) {
		t.Fatalf("asset flow hard block should be skipped")
	}
	base.Support = market.Zone{}
	if nearMissLayerCandidateOK(base) {
		t.Fatalf("invalid support should be skipped")
	}
}

func TestForcedNearMissAssetPlanCreatesSupportLayers(t *testing.T) {
	cfg := triggerAuditConfig()
	candidate := agent2.WatchCandidate{Symbol: "ETHUSDT", Price: 100, Support: market.Zone{Low: 90, High: 99}, RewardRisk: 2.5}
	plan := forcedNearMissAssetPlan(cfg, candidate, 0.05, 10)
	if plan.State != agent2.StateActiveLimit || len(plan.Layers) != len(cfg.Execution.LayerDistribution) {
		t.Fatalf("unexpected forced plan: %+v", plan)
	}
	if plan.Layers[0].Price != 99 || plan.Layers[1].Price != 94.5 || plan.Layers[2].Price != 90 {
		t.Fatalf("layers not based on support high/mid/low: %+v", plan.Layers)
	}
	if plan.Invalidation != 85.5 {
		t.Fatalf("invalidation=%v want 85.5", plan.Invalidation)
	}
}

func TestRunNearMissLayerAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": auditCandles("ETHUSDT", 140, 80), "SOLUSDT": auditCandles("SOLUSDT", 140, 60)}
	got, err := RunNearMissLayerAudit(cfg, btc, assets, NearMissLayerAuditConfig{ReadinessThresholds: []float64{0.30}, InvalidationBuffers: []float64{0.05}, TakeProfitPcts: []float64{0.05}, TimeStopDays: []int{0, 2}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" {
		t.Fatalf("expected enabled result with summary: %+v", got)
	}
}

func TestNearMissLayerVerdictCandidate(t *testing.T) {
	row := NearMissLayerAuditRow{PlansCreated: 1, OrdersPlaced: 3, FinalPnL: 10, MaxDrawdown: -0.03}
	if got := nearMissLayerAuditVerdict(row); got != NearMissLayerVerdictCandidate {
		t.Fatalf("verdict=%s want %s", got, NearMissLayerVerdictCandidate)
	}
	row.Invalidations = 1
	row.FinalPnL = -5
	if got := nearMissLayerAuditVerdict(row); got != NearMissLayerVerdictReject {
		t.Fatalf("verdict=%s want %s", got, NearMissLayerVerdictReject)
	}
}
