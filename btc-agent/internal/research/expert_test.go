package research

import (
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

func TestClassifyCategoryAndConfidence(t *testing.T) {
	cases := []struct {
		text     string
		source   string
		url      string
		category string
		want     float64
	}{
		{"Fed signals rate cut after CPI inflation release", "Federal Reserve", "https://federalreserve.gov/rss", "macro", 1},
		{"New tariff raises trade war risk", "Reuters", "https://reuters.com/feed", "trade", 0.9},
		{"SEC policy update", "CoinDesk", "https://coindesk.com/feed", "policy", 0.7},
		{"ETH ecosystem update", "Unknown", "https://example.com", "crypto", 0.3},
	}
	for _, tc := range cases {
		if got := classifyCategory(tc.text); got != tc.category {
			t.Fatalf("category(%q)=%s want %s", tc.text, got, tc.category)
		}
		if got := sourceConfidence(tc.source, tc.url); got != tc.want {
			t.Fatalf("confidence(%s)=%v want %v", tc.source, got, tc.want)
		}
	}
}

func TestComposeExpertReportKeepsEvidenceAndMarketContext(t *testing.T) {
	now := time.Now().UTC()
	brief := BriefResult{GeneratedAt: now, Summary: "brief ready", Items: []ResearchItem{
		{Source: "Federal Reserve", Title: "Fed holds rate", URL: "https://federalreserve.gov/a", PublishedAt: now, Category: "macro", Confidence: 1, Risk: RiskInfo},
		{Source: "Reuters", Title: "Tariff dispute", URL: "https://reuters.com/a", PublishedAt: now, Category: "trade", Confidence: 0.9, Risk: RiskWarn},
	}}
	analysis := agent1.MarketAnalysis{BTCPrice: 65000, MarketRegime: "RANGE", ActionPermission: agent1.Allowed}
	plan := agent2.Plan{State: agent2.StateActiveLimit}
	report := ComposeExpertReport(brief, analysis, plan, 4, 30)
	if len(report.Sections) != 3 {
		t.Fatalf("sections=%d want 3", len(report.Sections))
	}
	if len(report.Sections[0].Evidence) != 1 || report.Sections[0].Evidence[0].Confidence != 1 {
		t.Fatalf("macro evidence=%+v", report.Sections[0].Evidence)
	}
	if len(report.RiskSignals) != 1 || report.RiskSignals[0].Type != "trade" {
		t.Fatalf("risk signals=%+v", report.RiskSignals)
	}
}
