package learning

import (
	"strings"
	"testing"

	"btc-agent/internal/backtest"
)

func TestBuildRecommendationsFlowCandidate(t *testing.T) {
	result := BuildRecommendations(backtest.Result{FlowParamQualityAudit: backtest.FlowParamQualityAuditResult{Enabled: true, Rows: []backtest.FlowParamQualityAuditRow{
		{Name: "current", BullishCount: 10, BullishAvgReturn: map[int]float64{7: 0.01}, FalsePositiveRate: 0.30, Score: 0.01, Verdict: backtest.FlowParamQualityKeepCurrent},
		{Name: "sensitive", BullishCount: 20, AddedBullishCount: 8, BullishAvgReturn: map[int]float64{7: 0.03}, FalsePositiveRate: 0.20, Score: 0.03, Verdict: backtest.FlowParamQualityCandidateTune},
	}}})
	if len(result.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	got := result.Recommendations[0]
	if got.Area != AreaFlowParams || got.Severity != SeverityActionable {
		t.Fatalf("unexpected top recommendation: %+v", got)
	}
	if !strings.Contains(got.ManualAction, "manual review") {
		t.Fatalf("manual action should require manual review: %s", got.ManualAction)
	}
}

func TestBuildRecommendationsDataQualityFallback(t *testing.T) {
	result := BuildRecommendations(backtest.Result{})
	if len(result.Recommendations) != 1 {
		t.Fatalf("recommendations=%d want 1", len(result.Recommendations))
	}
	got := result.Recommendations[0]
	if got.Area != AreaDataQuality || got.Confidence != ConfidenceLow {
		t.Fatalf("unexpected fallback: %+v", got)
	}
}

func TestBuildRecommendationsOpportunityAudit(t *testing.T) {
	result := BuildRecommendations(backtest.Result{Agent2OpportunityAudit: backtest.Agent2OpportunityAuditResult{Enabled: true, Rows: []backtest.Agent2OpportunityAuditRow{{Symbol: "ETHUSDT", Samples: 20, NearMissCount: 12, TopMissingGate: "DISCOUNT_ZONE", RecommendedAction: "Many candidates are close to discount; review discount premium in backtest only before any config change.", ResearchOnlyVerdict: backtest.OpportunityVerdictTuneReview}}}})
	if len(result.Recommendations) == 0 || result.Recommendations[0].Severity != SeverityActionable {
		t.Fatalf("expected actionable opportunity recommendation: %+v", result.Recommendations)
	}
	text := result.Recommendations[0].Recommendation + " " + result.Recommendations[0].ManualAction
	if !strings.Contains(text, "no live config was changed") {
		t.Fatalf("recommendation must stay research-only: %+v", result.Recommendations[0])
	}
}

func TestBuildRecommendationsStrategyIntelligenceIsDiagnosticOnly(t *testing.T) {
	result := BuildRecommendations(backtest.Result{Agent2OpportunityAudit: backtest.Agent2OpportunityAuditResult{Enabled: true, Rows: []backtest.Agent2OpportunityAuditRow{{Symbol: "ETHUSDT", Samples: 12, TopMissingGate: "REWARD_RISK"}}}})
	found := false
	for _, rec := range result.Recommendations {
		if rec.Area != AreaStrategyIntelligence {
			continue
		}
		found = true
		text := rec.Recommendation + " " + rec.ManualAction
		for _, want := range []string{"Manual review required", "no live config changed", "no order authority changed", "WATCH/SCOUT/ARMED must not create orders"} {
			if !strings.Contains(text, want) {
				t.Fatalf("strategy intelligence recommendation missing %q: %+v", want, rec)
			}
		}
	}
	if !found {
		t.Fatalf("missing strategy intelligence recommendation: %+v", result.Recommendations)
	}
}

func TestBuildRecommendationsLayerAndExitCandidates(t *testing.T) {
	result := BuildRecommendations(backtest.Result{
		LayerAudit: backtest.LayerAuditResult{Enabled: true, Rows: []backtest.LayerAuditRow{{Symbol: "ETHUSDT", InvalidationBuffer: 0.03, LayerDepthMultiplier: 1.25, OrdersPlaced: 12, OrdersFilled: 8, FinalPnL: 20, MaxDrawdown: -0.05, Verdict: "CANDIDATE"}}},
		ExitAudit:  backtest.ExitAuditResult{Enabled: true, Rows: []backtest.ExitAuditRow{{Symbol: "ETHUSDT", TakeProfitPct: 0.05, TimeStopDays: 7, OrdersPlaced: 12, TakeProfits: 5, FinalPnL: 15, MaxDrawdown: -0.04, Verdict: "CANDIDATE"}}},
	})
	areas := map[string]bool{}
	for _, rec := range result.Recommendations {
		areas[rec.Area] = true
		text := strings.ToLower(rec.Recommendation + " " + rec.ManualAction)
		for _, unsafe := range []string{"place live", "submit order", "auto-tune", "auto tune", "override deterministic"} {
			if strings.Contains(text, unsafe) {
				t.Fatalf("recommendation contains unsafe phrase %q: %+v", unsafe, rec)
			}
		}
	}
	if !areas[AreaLayering] || !areas[AreaExit] {
		t.Fatalf("missing layer/exit recommendations: %+v", result.Recommendations)
	}
}
