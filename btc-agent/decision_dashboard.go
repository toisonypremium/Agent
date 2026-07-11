package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/reportio"
)

const decisionDashboardResearchOnly = "Research/report only: dashboard không đặt lệnh, không sửa config, không bypass ACTIVE_LIMIT."

type DecisionDashboardReport struct {
	GeneratedAt        time.Time         `json:"generated_at"`
	BotReady           bool              `json:"bot_ready"`
	MarketReady        bool              `json:"market_ready"`
	CanSubmitNow       bool              `json:"can_submit_now"`
	PlanState          agent2.State      `json:"plan_state,omitempty"`
	BTCPermission      agent1.Permission `json:"btc_permission,omitempty"`
	BestProductionCoin string            `json:"best_production_coin,omitempty"`
	BestUniverseCoin   string            `json:"best_universe_coin,omitempty"`
	NextTrigger        string            `json:"next_trigger,omitempty"`
	TechnicalSummary   string            `json:"technical_summary,omitempty"`
	CapitalSummary     string            `json:"capital_summary,omitempty"`
	FilterSummary      string            `json:"filter_summary,omitempty"`
	UniverseSummary    string            `json:"universe_summary,omitempty"`
	LiveSummary        string            `json:"live_summary,omitempty"`
	Blockers           []string          `json:"blockers,omitempty"`
	Actions            []string          `json:"actions,omitempty"`
	Safety             string            `json:"safety"`
	ResearchOnly       string            `json:"research_only"`
}

func writeDecisionDashboardReport(snapshot BotRuntimeSnapshot, scenario ScenarioReport, technical TechnicalScorecardReport, capital CapitalPlanResearchReport, filter FilterAttributionReport) error {
	universe, _ := loadUniverseResearchReportFile()
	return writeDecisionDashboardReportFile(buildDecisionDashboard(snapshot, scenario, technical, capital, filter, universe))
}

func writeDecisionDashboardReportFile(report DecisionDashboardReport) error {
	if err := reportio.WriteJSON("reports", "decision_dashboard_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "decision_dashboard_latest.md"), []byte(decisionDashboardMarkdown(report)), 0600)
}

func buildDecisionDashboard(snapshot BotRuntimeSnapshot, scenario ScenarioReport, technical TechnicalScorecardReport, capital CapitalPlanResearchReport, filter FilterAttributionReport, universe agent2.UniverseResearchReport) DecisionDashboardReport {
	report := DecisionDashboardReport{GeneratedAt: snapshot.GeneratedAt, PlanState: snapshot.PlanState, BTCPermission: snapshot.BTCPermission, CanSubmitNow: snapshot.CanSubmitLiveOrder, Safety: safetyLine, ResearchOnly: decisionDashboardResearchOnly}
	report.BotReady = snapshot.Mode == "live-auto" && snapshot.AutoLiveAllowed && snapshot.LiveEnabled && snapshot.AutoExecute && snapshot.RealTradingEnabled && !snapshot.RequireManualConfirm && !snapshot.ProofOnly && !snapshot.OperatorHalt && snapshot.DoctorStatus != string(liveguard.DoctorBlock)
	report.MarketReady = snapshot.PlanState == agent2.StateActiveLimit && snapshot.BTCPermission == agent1.Allowed
	report.BestProductionCoin = bestTechnicalCoin(technical)
	report.BestUniverseCoin = bestUniverseCoin(universe)
	report.TechnicalSummary = technical.Summary
	report.CapitalSummary = capital.Summary
	report.FilterSummary = filter.Summary
	report.UniverseSummary = emptyStringDefault(universe.Summary, "universe research not run yet")
	report.LiveSummary = snapshot.ManagedStatus
	if report.LiveSummary == "" {
		report.LiveSummary = snapshot.SupervisorSummary
	}
	report.Blockers = uniqueStringsMain(append([]string{}, scenario.Blockers...))
	if !report.BotReady {
		report.Blockers = append(report.Blockers, "bot runtime chưa đủ điều kiện live-auto đầy đủ")
	}
	if !report.MarketReady {
		report.Blockers = append(report.Blockers, "market chưa ACTIVE_LIMIT + ALLOWED")
	}
	report.Blockers = uniqueStringsMain(report.Blockers)
	report.NextTrigger = dashboardNextTrigger(scenario, technical)
	report.Actions = dashboardActions(report)
	return report
}

func bestTechnicalCoin(report TechnicalScorecardReport) string {
	best := TechnicalScorecardCoin{}
	for _, coin := range report.Coins {
		if best.Symbol == "" || coin.TechnicalScore > best.TechnicalScore {
			best = coin
		}
	}
	return best.Symbol
}

func bestUniverseCoin(report agent2.UniverseResearchReport) string {
	for _, row := range report.TopCandidates {
		if row.DataStatus == agent2.UniverseDataOK && !agent2.OpportunityCompositeBlocked(row.OpportunityVerdict) {
			return row.Symbol
		}
	}
	return ""
}

func dashboardNextTrigger(scenario ScenarioReport, technical TechnicalScorecardReport) string {
	if len(scenario.NearTriggers) > 0 {
		return scenario.NearTriggers[0]
	}
	for _, coin := range technical.Coins {
		if coin.NextTrigger != "" {
			return coin.Symbol + ": " + coin.NextTrigger
		}
	}
	if len(scenario.BTC.UnlockConditions) > 0 {
		return scenario.BTC.UnlockConditions[0]
	}
	return "Chờ ACTIVE_LIMIT + ALLOWED và safety gates pass."
}

func dashboardActions(report DecisionDashboardReport) []string {
	if report.CanSubmitNow {
		return []string{"Bot có thể submit theo managed cycle: spot limit BUY post-only, qua preflight/caps/MM gate."}
	}
	actions := []string{"Đứng ngoài; không đặt lệnh khi chưa ACTIVE_LIMIT + ALLOWED."}
	if !report.MarketReady {
		actions = append(actions, "Chờ BTC permission ALLOWED và plan ACTIVE_LIMIT.")
	}
	if report.BestProductionCoin != "" {
		actions = append(actions, "Theo dõi production coin tốt nhất: "+report.BestProductionCoin+".")
	}
	if report.BestUniverseCoin != "" {
		actions = append(actions, "Universe research gợi ý theo dõi: "+report.BestUniverseCoin+"; không tự thay production config.")
	}
	return actions
}

func loadUniverseResearchReportFile() (agent2.UniverseResearchReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "coin_universe_research_latest.json"))
	if err != nil {
		return agent2.UniverseResearchReport{}, false
	}
	var out agent2.UniverseResearchReport
	if err := json.Unmarshal(b, &out); err != nil {
		return agent2.UniverseResearchReport{}, false
	}
	return out, true
}

func decisionDashboardMarkdown(report DecisionDashboardReport) string {
	var b strings.Builder
	b.WriteString("DECISION DASHBOARD\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString(fmt.Sprintf("Bot ready: %v | Market ready: %v | Can submit now: %v\n", report.BotReady, report.MarketReady, report.CanSubmitNow))
	b.WriteString(fmt.Sprintf("Plan: %s | BTC permission: %s\n", report.PlanState, report.BTCPermission))
	b.WriteString(fmt.Sprintf("Best production coin: %s | Best universe coin: %s\n", emptyStringDefault(report.BestProductionCoin, "n/a"), emptyStringDefault(report.BestUniverseCoin, "n/a")))
	b.WriteString("Next trigger: " + emptyStringDefault(report.NextTrigger, "n/a") + "\n\n")
	b.WriteString("Summaries:\n")
	b.WriteString("- Technical: " + emptyStringDefault(report.TechnicalSummary, "n/a") + "\n")
	b.WriteString("- Capital: " + emptyStringDefault(report.CapitalSummary, "n/a") + "\n")
	b.WriteString("- Filters: " + emptyStringDefault(report.FilterSummary, "n/a") + "\n")
	b.WriteString("- Universe: " + emptyStringDefault(report.UniverseSummary, "n/a") + "\n")
	b.WriteString("- Live: " + emptyStringDefault(report.LiveSummary, "n/a") + "\n")
	if len(report.Blockers) > 0 {
		b.WriteString("\nBlockers:\n")
		for _, blocker := range firstStrings(report.Blockers, 8) {
			b.WriteString("- " + blocker + "\n")
		}
	}
	if len(report.Actions) > 0 {
		b.WriteString("\nActions:\n")
		for _, action := range report.Actions {
			b.WriteString("- " + action + "\n")
		}
	}
	b.WriteString("\nSafety: " + report.Safety + "\n")
	b.WriteString(report.ResearchOnly + "\n")
	return b.String()
}
