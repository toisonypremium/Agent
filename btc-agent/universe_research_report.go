package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"btc-agent/internal/agent2"
	"btc-agent/internal/reportio"
)

func writeUniverseResearchReport(report agent2.UniverseResearchReport) error {
	if err := reportio.WriteJSON("reports", "coin_universe_research_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "coin_universe_research_latest.md"), []byte(universeResearchMarkdown(report)), 0600)
}

func universeResearchMarkdown(report agent2.UniverseResearchReport) string {
	var b strings.Builder
	b.WriteString("COIN UNIVERSE RESEARCH\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString("Summary: " + report.Summary + "\n")
	b.WriteString("Production assets: " + strings.Join(report.ProductionAssets, ", ") + "\n")
	b.WriteString("Universe: " + strings.Join(report.Universe, ", ") + "\n\n")
	if len(report.TopCandidates) > 0 {
		b.WriteString("Top candidates:\n")
		for _, row := range firstUniverseRows(report.TopCandidates, 10) {
			b.WriteString(fmt.Sprintf("- %s score=%.1f verdict=%s state=%s production=%v data=%s setup=%.2f RR=%.2f top=%s\n", row.Symbol, row.OpportunityScore, row.OpportunityVerdict, row.State, row.InProduction, row.DataStatus, row.SetupScore, row.RewardRisk, emptyStringDefault(row.TopBlockerKey, "none")))
			if row.NextTrigger != "" {
				b.WriteString("  next=" + row.NextTrigger + "\n")
			}
		}
		b.WriteString("\n")
	}
	if len(report.Rows) > 0 {
		b.WriteString("All rows:\n")
		for _, row := range report.Rows {
			b.WriteString(fmt.Sprintf("- %s state=%s opportunity=%.1f technical=%.0f%% verdict=%s data=%s reason=%s\n", row.Symbol, row.State, row.OpportunityScore, row.TechnicalScore*100, row.OpportunityVerdict, row.DataStatus, emptyStringDefault(row.Reason, row.TopBlocker)))
		}
	}
	b.WriteString("\nSafety: " + report.Safety + "\n")
	b.WriteString(report.ResearchOnly + "\n")
	return b.String()
}

func firstUniverseRows(items []agent2.UniverseResearchRow, limit int) []agent2.UniverseResearchRow {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
