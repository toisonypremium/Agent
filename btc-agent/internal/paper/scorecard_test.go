package paper

import (
	"testing"
	"time"

	"btc-agent/internal/agent2"
)

func TestBuildScorecardClassifiesPaperLifecycle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	orders := []agent2.PaperOrder{
		{ID: "open", Symbol: "ethusdt", Status: StatusOpen, Timestamp: now.Add(-time.Hour)},
		{ID: "fill", Symbol: "ETHUSDT", Status: StatusFilled},
		{ID: "invalid", Symbol: "SOLUSDT", Status: StatusInvalidated},
		{ID: "expire", Symbol: "SOLUSDT", Status: StatusExpired},
	}
	report := BuildScorecard(now, orders)
	if report.Readiness != "PAPER_EVIDENCE_ONLY" || report.TotalOrders != 4 || report.TerminalOrders != 3 || report.OpenOrders != 1 {
		t.Fatalf("unexpected scorecard: %+v", report)
	}
	if report.FilledOrders != 1 || report.InvalidatedOrders != 1 || report.ExpiredOrders != 1 || report.FillRate != 1.0/3.0 || report.InvalidationRate != 1.0/3.0 {
		t.Fatalf("unexpected metrics: %+v", report)
	}
	if report.AverageOpenAge != time.Hour || len(report.BySymbol) != 2 || report.BySymbol[0].Symbol != "ETHUSDT" {
		t.Fatalf("unexpected dimensions: %+v", report)
	}
}

func TestBuildScorecardRequiresEvidence(t *testing.T) {
	report := BuildScorecard(time.Now(), nil)
	if report.Readiness != "INSUFFICIENT_EVIDENCE" || len(report.Blockers) == 0 {
		t.Fatalf("expected evidence blocker: %+v", report)
	}
}
