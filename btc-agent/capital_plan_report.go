package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/reportio"
)

const capitalPlanResearchOnly = "Research only: không sửa config allocation, không bypass ACTIVE_LIMIT, WATCH/SCOUT/ARMED không tạo normal live order."

type CapitalPlanResearchReport struct {
	GeneratedAt      time.Time                 `json:"generated_at"`
	PlanState        agent2.State              `json:"plan_state,omitempty"`
	BTCPermission    string                    `json:"btc_permission,omitempty"`
	TotalCapital     float64                   `json:"total_capital"`
	ReserveCashRatio float64                   `json:"reserve_cash_ratio"`
	Coins            []CapitalPlanResearchCoin `json:"coins,omitempty"`
	Summary          string                    `json:"summary"`
	Safety           string                    `json:"safety"`
	ResearchOnly     string                    `json:"research_only"`
}

type CapitalPlanResearchCoin struct {
	Symbol                      string       `json:"symbol"`
	State                       agent2.State `json:"state"`
	CurrentConfigAllocation     float64      `json:"current_config_allocation"`
	SuggestedResearchAllocation float64      `json:"suggested_research_allocation"`
	MaxResearchNotional         float64      `json:"max_research_notional"`
	OpportunityScore            float64      `json:"opportunity_score"`
	OpportunityVerdict          string       `json:"opportunity_verdict"`
	QualityGrade                string       `json:"quality_grade,omitempty"`
	SuggestedLayers             int          `json:"suggested_layers"`
	RiskNote                    string       `json:"risk_note"`
	Reason                      string       `json:"reason"`
}

func writeCapitalPlanResearchReportFile(report CapitalPlanResearchReport) error {
	if err := reportio.WriteJSON("reports", "capital_plan_research_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "capital_plan_research_latest.md"), []byte(capitalPlanResearchMarkdown(report)), 0600)
}

func buildCapitalPlanResearchReport(cfg config.Config, snapshot BotRuntimeSnapshot) CapitalPlanResearchReport {
	report := CapitalPlanResearchReport{GeneratedAt: snapshot.GeneratedAt, PlanState: snapshot.PlanState, BTCPermission: string(snapshot.BTCPermission), TotalCapital: cfg.Portfolio.TotalCapital, ReserveCashRatio: cfg.Portfolio.ReserveCashRatio, Safety: safetyLine, ResearchOnly: capitalPlanResearchOnly}
	quality := qualityBySymbolFromSnapshot(snapshot)
	weights := map[string]float64{}
	composites := map[string]agent2.OpportunityComposite{}
	assetsBySymbol := map[string]agent2.AssetPlan{}
	for _, asset := range snapshot.Plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		assetsBySymbol[symbol] = asset
		composite := agent2.BuildOpportunityComposite(asset)
		composites[symbol] = composite
		if !agent2.OpportunityCompositeBlocked(composite.Verdict) && composite.Score >= 45 {
			weights[symbol] = composite.Score
		}
	}
	totalWeight := 0.0
	for _, weight := range weights {
		totalWeight += weight
	}
	allocatable := 1 - cfg.Portfolio.ReserveCashRatio
	if allocatable < 0 {
		allocatable = 0
	}
	for _, configured := range cfg.Data.Symbols.Assets {
		symbol := strings.ToUpper(strings.TrimSpace(configured))
		asset := assetsBySymbol[symbol]
		composite := composites[symbol]
		current := cfg.Portfolio.Allocation[symbol]
		if current <= 0 {
			current = cfg.Portfolio.Allocation[configured]
		}
		suggested := 0.0
		var reason string
		if totalWeight > 0 && weights[symbol] > 0 {
			suggested = allocatable * weights[symbol] / totalWeight
			reason = fmt.Sprintf("score-weighted research allocation %.1f%% từ opportunity %.1f", suggested*100, composite.Score)
		} else if totalWeight == 0 && !agent2.OpportunityCompositeBlocked(composite.Verdict) {
			suggested = current
			reason = "chưa đủ bằng chứng để đổi phân bổ; giữ allocation config trong research plan"
		} else if agent2.OpportunityCompositeBlocked(composite.Verdict) {
			reason = "blocked research allocation: " + composite.Reason
		} else {
			reason = "opportunity score dưới ngưỡng phân bổ research; suggested allocation=0"
		}
		row := CapitalPlanResearchCoin{Symbol: symbol, State: asset.State, CurrentConfigAllocation: current, SuggestedResearchAllocation: suggested, MaxResearchNotional: cfg.Portfolio.TotalCapital * suggested, OpportunityScore: composite.Score, OpportunityVerdict: composite.Verdict, QualityGrade: quality[symbol], SuggestedLayers: suggestedResearchLayers(composite.Score, composite.Verdict), RiskNote: capitalRiskNote(composite, asset), Reason: reason}
		report.Coins = append(report.Coins, row)
	}
	report.Summary = capitalPlanResearchSummary(report)
	return report
}

func qualityBySymbolFromSnapshot(snapshot BotRuntimeSnapshot) map[string]string {
	return map[string]string{}
}

func suggestedResearchLayers(score float64, verdict string) int {
	if agent2.OpportunityCompositeBlocked(verdict) {
		return 0
	}
	switch {
	case score >= 80:
		return 3
	case score >= 65:
		return 2
	case score >= 45:
		return 1
	default:
		return 0
	}
}

func capitalRiskNote(composite agent2.OpportunityComposite, asset agent2.AssetPlan) string {
	if agent2.OpportunityCompositeBlocked(composite.Verdict) {
		return "không đề xuất phân bổ vốn nghiên cứu vì blocker/risk"
	}
	if asset.State != agent2.StateActiveLimit {
		return "chỉ chuẩn bị vốn nghiên cứu; live vẫn chờ ACTIVE_LIMIT + ALLOWED"
	}
	return "asset ACTIVE_LIMIT; live vẫn phải qua BTC permission, preflight, caps, MM gate"
}

func capitalPlanResearchSummary(report CapitalPlanResearchReport) string {
	total := 0.0
	best := CapitalPlanResearchCoin{}
	for _, coin := range report.Coins {
		total += coin.SuggestedResearchAllocation
		if best.Symbol == "" || coin.OpportunityScore > best.OpportunityScore {
			best = coin
		}
	}
	if best.Symbol == "" {
		return "Capital research plan coins=0"
	}
	return fmt.Sprintf("Capital research plan coins=%d suggested_total=%.1f%% best=%s score=%.1f verdict=%s", len(report.Coins), total*100, best.Symbol, best.OpportunityScore, best.OpportunityVerdict)
}

func capitalPlanResearchMarkdown(report CapitalPlanResearchReport) string {
	var b strings.Builder
	b.WriteString("CAPITAL PLAN RESEARCH\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString("Summary: " + report.Summary + "\n")
	b.WriteString(fmt.Sprintf("Plan state: %s | BTC permission: %s\n", report.PlanState, report.BTCPermission))
	b.WriteString(fmt.Sprintf("Total capital: %.2f | reserve cash: %.1f%%\n\n", report.TotalCapital, report.ReserveCashRatio*100))
	for _, coin := range report.Coins {
		b.WriteString(fmt.Sprintf("- %s state=%s current=%.1f%% suggested=%.1f%% max_notional=%.2f score=%.1f verdict=%s layers=%d\n", coin.Symbol, coin.State, coin.CurrentConfigAllocation*100, coin.SuggestedResearchAllocation*100, coin.MaxResearchNotional, coin.OpportunityScore, coin.OpportunityVerdict, coin.SuggestedLayers))
		if coin.RiskNote != "" {
			b.WriteString("  risk=" + coin.RiskNote + "\n")
		}
		if coin.Reason != "" {
			b.WriteString("  reason=" + coin.Reason + "\n")
		}
	}
	b.WriteString("\nSafety: " + report.Safety + "\n")
	b.WriteString(report.ResearchOnly + "\n")
	return b.String()
}
