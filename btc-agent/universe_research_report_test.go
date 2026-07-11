package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
)

func TestUniverseResearchMarkdownIsResearchOnly(t *testing.T) {
	report := agent2.UniverseResearchReport{GeneratedAt: time.Unix(1700000000, 0), ProductionAssets: []string{"ETHUSDT"}, Universe: []string{"ETHUSDT", "LINKUSDT"}, Summary: "Universe research symbols=2", Safety: safetyLine, ResearchOnly: agent2.UniverseResearchOnly, Rows: []agent2.UniverseResearchRow{{Symbol: "ETHUSDT", DataStatus: agent2.UniverseDataOK, OpportunityScore: 70, OpportunityVerdict: agent2.OpportunityVerdictNormal}}}
	md := universeResearchMarkdown(report)
	for _, want := range []string{"COIN UNIVERSE RESEARCH", "Research only", "không bypass ACTIVE_LIMIT", safetyLine} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}
