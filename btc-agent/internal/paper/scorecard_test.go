package paper

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
)

func TestBuildScorecardClassifiesPaperLifecycle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	orders := []agent2.PaperOrder{
		{ID: "open", Symbol: "ethusdt", Status: StatusOpen, Timestamp: now.Add(-time.Hour)},
		{ID: "older-open", Symbol: "ETHUSDT", Status: StatusOpen, Timestamp: now.Add(-3 * time.Hour)},
		{ID: "fill", Symbol: "ETHUSDT", Status: StatusFilled},
		{ID: "invalid", Symbol: "SOLUSDT", Status: StatusInvalidated},
		{ID: "expire", Symbol: "SOLUSDT", Status: StatusExpired},
	}
	report := BuildScorecard(now, orders)
	if report.Readiness != "PAPER_EVIDENCE_ONLY" || report.TotalOrders != 5 || report.TerminalOrders != 3 || report.OpenOrders != 2 {
		t.Fatalf("unexpected scorecard: %+v", report)
	}
	if report.FilledOrders != 1 || report.InvalidatedOrders != 1 || report.ExpiredOrders != 1 || report.FillRate != 1.0/3.0 || report.InvalidationRate != 1.0/3.0 {
		t.Fatalf("unexpected metrics: %+v", report)
	}
	if report.AverageOpenAge != 2*time.Hour || report.MaximumOpenAge != 3*time.Hour || report.UnknownStatuses != 0 || len(report.BySymbol) != 2 || report.BySymbol[0].Symbol != "ETHUSDT" {
		t.Fatalf("unexpected dimensions: %+v", report)
	}
}

func TestBuildScorecardRequiresEvidence(t *testing.T) {
	report := BuildScorecard(time.Now(), nil)
	if report.Readiness != "INSUFFICIENT_EVIDENCE" || len(report.Blockers) == 0 {
		t.Fatalf("expected evidence blocker: %+v", report)
	}
}

func TestBuildScorecardUnknownStatusBlocksEvidence(t *testing.T) {
	report := BuildScorecard(time.Now(), []agent2.PaperOrder{{ID: "unknown", Symbol: "ETHUSDT", Status: "PENDING_REVIEW"}})
	if report.Readiness != "INSUFFICIENT_EVIDENCE" || report.UnknownStatuses != 1 {
		t.Fatalf("unknown status must block review: %+v", report)
	}
	if !strings.Contains(ScorecardMarkdown(report), "unknown_statuses=1") {
		t.Fatalf("markdown omitted unknown status metric")
	}
}
