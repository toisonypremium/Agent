package research

import (
	"fmt"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

func ComposeExpertReport(brief BriefResult, analysis agent1.MarketAnalysis, plan agent2.Plan, maxSections int, maxItems int) ExpertReport {
	if maxSections <= 0 {
		maxSections = 4
	}
	if maxItems <= 0 {
		maxItems = 30
	}
	report := ExpertReport{GeneratedAt: brief.GeneratedAt, ResearchSummary: brief.Summary}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = analysis.Timestamp
	}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = plan.Timestamp
	}

	byCategory := map[string][]ResearchItem{}
	for _, item := range brief.Items {
		category := item.Category
		if category == "" {
			category = "crypto"
		}
		byCategory[category] = append(byCategory[category], item)
	}

	for _, category := range []string{"macro", "policy", "trade"} {
		if len(report.Sections) >= maxSections {
			break
		}
		items := limitedItems(byCategory[category], maxItems)
		if len(items) == 0 {
			continue
		}
		report.Sections = append(report.Sections, Section{
			Title:    expertSectionTitle(category),
			Content:  deterministicCategorySummary(category, items),
			Evidence: evidencePoints(items),
		})
		report.RiskSignals = append(report.RiskSignals, categoryRiskSignals(category, items)...)
	}

	market := fmt.Sprintf("BTC %.2f | Regime %s | Permission %s | Accumulation %s | Risk %s | Falling knife %s | FOMO %s. Plan: %s.",
		analysis.BTCPrice, analysis.MarketRegime, analysis.ActionPermission, analysis.BTCAccumulation.Phase, analysis.RiskLevel, analysis.FallingKnifeRisk, analysis.FomoRisk, plan.State)
	if len(report.Sections) < maxSections {
		report.Sections = append(report.Sections, Section{Title: "📊 THỊ TRƯỜNG & KỊCH BẢN BOT", Content: market})
	}
	if analysis.FallingKnifeRisk == agent1.High || analysis.RiskLevel == agent1.High {
		report.RiskSignals = append(report.RiskSignals, RiskSignal{Type: "market", Level: "HIGH", Detail: "BTC deterministic risk gate is high; no new accumulation authority.", Impact: "btc,altcoin"})
	}
	report.Summary = fmt.Sprintf("Expert evidence report: macro=%d policy=%d trade=%d crypto=%d | BTC=%s/%s | plan=%s", len(byCategory["macro"]), len(byCategory["policy"]), len(byCategory["trade"]), len(byCategory["crypto"]), analysis.MarketRegime, analysis.ActionPermission, plan.State)
	report.RefreshSummary()
	return report
}

func limitedItems(items []ResearchItem, max int) []ResearchItem {
	if len(items) <= max {
		return items
	}
	return items[:max]
}

func expertSectionTitle(category string) string {
	switch category {
	case "macro":
		return "🌍 KINH TẾ VĨ MÔ & LÃI SUẤT"
	case "policy":
		return "⚖️ CHÍNH SÁCH & PHÁP LÝ"
	case "trade":
		return "🌐 THƯƠNG MẠI & ĐỊA CHÍNH TRỊ"
	default:
		return "📰 THÔNG TIN THỊ TRƯỜNG"
	}
}

func deterministicCategorySummary(category string, items []ResearchItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Title)
	}
	return fmt.Sprintf("%d evidence items classified as %s: %s", len(items), category, strings.Join(parts, " | "))
}

func evidencePoints(items []ResearchItem) []EvidencePoint {
	out := make([]EvidencePoint, 0, len(items))
	for _, item := range items {
		out = append(out, EvidencePoint{Source: item.Source, Headline: item.Title, URL: item.URL, Published: item.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"), Confidence: item.Confidence, Relevance: "context"})
	}
	return out
}

func categoryRiskSignals(category string, items []ResearchItem) []RiskSignal {
	out := []RiskSignal{}
	for _, item := range items {
		if item.Risk != RiskWarn {
			continue
		}
		impact := "global"
		if category == "policy" {
			impact = "crypto"
		}
		if category == "macro" {
			impact = "btc,altcoin"
		}
		out = append(out, RiskSignal{Type: category, Level: "MEDIUM", Detail: item.Title, Impact: impact})
	}
	return out
}
