package survey

import (
	"strings"
	"testing"

	"btc-agent/internal/backtest"
	"btc-agent/internal/liveguard"
)

func TestBuildLowSampleStaysReportOnly(t *testing.T) {
	got := Build(backtest.Result{WindowsTested: 5}, nil)
	if got.DataCoverage.Confidence != ConfidenceLow {
		t.Fatalf("confidence=%s want LOW", got.DataCoverage.Confidence)
	}
	for _, action := range got.LearningActions {
		if action.Severity == SeverityActionableReview {
			t.Fatalf("low sample should not be actionable: %+v", action)
		}
	}
	md := Markdown(got)
	for _, want := range []string{"does not write config", "does not place", "No real order was placed"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestBuildUsesOpportunityAndManagedHistory(t *testing.T) {
	bt := backtest.Result{
		WindowsTested: 140,
		Agent2OpportunityAudit: backtest.Agent2OpportunityAuditResult{Enabled: true, Rows: []backtest.Agent2OpportunityAuditRow{{
			Symbol: "ETHUSDT", Samples: 40, NearMissCount: 22, TopMissingGate: "DISCOUNT_ZONE", ResearchOnlyVerdict: backtest.OpportunityVerdictTuneReview,
		}}},
	}
	history := liveguard.LiveManagerHistoryResult{WindowsTested: 120, Total: liveguard.LiveManagerHistoryStats{Placed: 20, Filled: 12, FillRate: 0.60, CancelRate: 0.10, QualityGrade: "A", QualityScore: 72}}
	got := Build(bt, &history)
	if got.Agent2Gate.Verdict != SeverityActionableReview {
		t.Fatalf("agent2 verdict=%s want actionable", got.Agent2Gate.Verdict)
	}
	if got.ManagedLive.Verdict != SeverityActionableReview {
		t.Fatalf("managed verdict=%s want actionable", got.ManagedLive.Verdict)
	}
	if len(got.LearningActions) == 0 {
		t.Fatalf("expected learning actions")
	}
}
