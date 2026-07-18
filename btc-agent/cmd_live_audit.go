package main

import (
	"encoding/json"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/microstructure"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	AuditApprovedMonitoring = "APPROVED_MONITORING"
	AuditApprovedDryRun     = "APPROVED_DRY_RUN"
	AuditApprovedRealOrder  = "APPROVED_REAL_ORDER"
	AuditBlocked            = "BLOCKED"

	MarketAuthorityBlocked        = "BLOCKED"
	MarketAuthorityDryRunReady    = "DRY_RUN_READY"
	MarketAuthorityRealOrderReady = "REAL_ORDER_READY"
)

type liveAutoAuditReport struct {
	GeneratedAt                  time.Time                        `json:"generated_at"`
	Verdict                      string                           `json:"verdict"`
	Monitoring                   bool                             `json:"monitoring_approved"`
	DryRun                       bool                             `json:"dry_run_approved"`
	RealOrder                    bool                             `json:"real_order_approved"`
	DryRunInfrastructureApproved bool                             `json:"dry_run_infrastructure_approved"`
	CurrentMarketAuthority       string                           `json:"current_market_authority"`
	CurrentDryRunApproved        bool                             `json:"current_dry_run_approved"`
	CurrentDryRun                liveguard.ManagedCycleResult     `json:"current_dry_run"`
	Reasons                      []string                         `json:"reasons,omitempty"`
	Doctor                       liveguard.RuntimeDoctorResult    `json:"doctor"`
	Analysis                     agent1.MarketAnalysis            `json:"analysis"`
	Plan                         agent2.Plan                      `json:"plan"`
	Microstructure               microstructure.Summary           `json:"microstructure"`
	Proof                        liveguard.Proof                  `json:"proof"`
	Desired                      []liveguard.ManagedDesiredOrder  `json:"desired,omitempty"`
	Blocked                      []liveguard.ManagedOrderDecision `json:"blocked,omitempty"`
	FinalAssertions              []string                         `json:"final_assertions,omitempty"`
	ForcedSimulation             liveguard.ForcedSimulationResult `json:"forced_simulation"`
	OpenOrders                   []live.OrderStatus               `json:"open_orders,omitempty"`
	Positions                    []live.LivePosition              `json:"positions,omitempty"`
	Summary                      string                           `json:"summary"`
}

func runLiveAutoAudit(ctx context.Context, cfg config.Config, db *storage.DB) error {
	report := buildLiveAutoAudit(ctx, cfg, db)
	if err := saveJSONFile("reports", "live_auto_audit_latest.json", report); err != nil {
		return err
	}
	md := liveAutoAuditMarkdown(report)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_auto_audit_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func buildLiveAutoAudit(ctx context.Context, cfg config.Config, db *storage.DB) liveAutoAuditReport {
	now := time.Now().UTC()
	report := liveAutoAuditReport{GeneratedAt: now, Verdict: AuditBlocked, CurrentMarketAuthority: MarketAuthorityBlocked}
	report.Doctor = buildLiveDoctorResult(ctx, cfg, db)
	analysis, analysisErr := db.LatestAnalysis()
	if analysisErr != nil {
		report.Reasons = append(report.Reasons, "latest analysis unavailable: "+analysisErr.Error())
	} else {
		report.Analysis = analysis
	}
	plan, planErr := db.LatestPlan()
	if planErr != nil {
		report.Reasons = append(report.Reasons, "latest plan unavailable: "+planErr.Error())
	} else {
		report.Plan = plan
	}
	report.Microstructure = latestMicrostructureSummary(cfg, db, now)
	open, openErr := db.OpenLiveOrdersDetailed()
	if openErr != nil {
		report.Reasons = append(report.Reasons, "open live orders unavailable: "+openErr.Error())
	} else {
		report.OpenOrders = open
	}
	positions, posErr := db.LivePositions()
	if posErr != nil {
		report.Reasons = append(report.Reasons, "live positions unavailable: "+posErr.Error())
	} else {
		report.Positions = positions
	}

	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	if cfg.Live.Enabled && strings.ToLower(cfg.Live.Exchange) == "okx" {
		if client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv); err == nil {
			balanceReader = client
			filterReader = client
		}
	}
	if planErr == nil {
		report.Proof = liveguard.BuildProofWithChecks(ctx, cfg, plan, balanceReader, filterReader)
		execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: string(report.Analysis.BTCAccumulation.Phase), FirstOrderDryRunApproved: true}
		applyPortfolioLossState(cfg, db, &execCtx, time.Now())
		if hasHistory, err := db.HasManagedRealOrderSubmission(); err == nil {
			execCtx.ManagedOrderHistoryKnown = true
			execCtx.HasManagedRealOrderHistory = hasHistory
		}
		report.CurrentDryRun = liveguard.ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, open, positions, nil, nil, nil, db, execCtx, nil, true)
		report.Desired = report.CurrentDryRun.Desired
		report.Blocked = report.CurrentDryRun.Blocked
		report.CurrentDryRunApproved = currentDryRunReady(report.CurrentDryRun)
		if len(report.Desired) > 0 {
			total, bySymbol := liveguardAuditOpenNotional(open)
			blockers := liveguard.AssertManagedExecutionAllowed(liveguard.ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: report.Desired[0], OpenNotionalTotal: total, OpenNotionalBySymbol: bySymbol, DryRun: false, ManagedExecutionContext: execCtx})
			report.FinalAssertions = liveguard.FinalAssertionAuditWithContext(execCtx, plan, report.Desired[0], blockers)
		}
	}
	report.ForcedSimulation = liveguard.RunForcedActiveLimitSimulation(cfg)

	report.Monitoring = report.Doctor.Status != liveguard.DoctorBlock || onlyMarketNotReady(report.Doctor.Blockers)
	report.DryRunInfrastructureApproved = report.Monitoring && cfg.Live.Enabled && cfg.Live.SupervisorEnabled && cfg.Live.OrderManagementEnabled && report.ForcedSimulation.Passed
	report.DryRun = report.DryRunInfrastructureApproved
	realReasons := realOrderAuditBlockers(report)
	if len(realReasons) == 0 {
		report.RealOrder = true
		report.CurrentMarketAuthority = MarketAuthorityRealOrderReady
		report.Verdict = AuditApprovedRealOrder
	} else {
		report.Reasons = append(report.Reasons, realReasons...)
		if report.CurrentDryRunApproved {
			report.CurrentMarketAuthority = MarketAuthorityDryRunReady
		}
		if report.DryRun {
			report.Verdict = AuditApprovedDryRun
		} else if report.Monitoring {
			report.Verdict = AuditApprovedMonitoring
		}
	}
	report.Reasons = uniqueStringsMain(report.Reasons)
	report.Summary = fmt.Sprintf("%s: monitoring=%v dry_run=%v real_order=%v reasons=%d", report.Verdict, report.Monitoring, report.DryRun, report.RealOrder, len(report.Reasons))
	return report
}

func currentDryRunReady(result liveguard.ManagedCycleResult) bool {
	if result.Status != liveguard.ManagedCycleDryRun {
		return false
	}
	if len(result.Placed) == 0 {
		return false
	}
	for _, item := range result.Blocked {
		if strings.Contains(item.Reason, "final execution assertion blocked") {
			return false
		}
	}
	return true
}

func liveguardAuditOpenNotional(open []live.OrderStatus) (float64, map[string]float64) {
	total := 0.0
	bySymbol := map[string]float64{}
	for _, order := range open {
		notional := order.Notional
		if notional <= 0 && order.Price > 0 && order.Quantity > 0 {
			notional = order.Price * order.Quantity
		}
		total += notional
		symbol := strings.ToUpper(order.Symbol)
		if symbol == "" {
			symbol = live.InternalSymbol(order.InstID)
		}
		bySymbol[symbol] += notional
	}
	return total, bySymbol
}

func onlyMarketNotReady(blockers []string) bool {
	if len(blockers) == 0 {
		return true
	}
	for _, blocker := range blockers {
		lower := strings.ToLower(blocker)
		if !strings.Contains(lower, "no deterministic") && !strings.Contains(lower, "plan") {
			return false
		}
	}
	return true
}

func realOrderAuditBlockers(r liveAutoAuditReport) []string {
	reasons := []string{}
	if r.Doctor.Status != liveguard.DoctorOK {
		reasons = append(reasons, "doctor not OK: "+r.Doctor.Status)
	}
	if r.Doctor.DataHealth.Status != "" && r.Doctor.DataHealth.Status == liveguard.DataHealthBlock {
		reasons = append(reasons, "data health block")
	}
	if r.Doctor.ReconcileSafety.Status != "" && r.Doctor.ReconcileSafety.Status == liveguard.ReconcileBlock {
		reasons = append(reasons, "reconcile block")
	}
	if r.Doctor.RiskGovernor.Status == liveguard.RiskGovernorBlock {
		reasons = append(reasons, "risk governor block")
	}
	if r.Microstructure.Enabled && r.Microstructure.Status != microstructure.StatusOK {
		reasons = append(reasons, "microstructure not OK: "+r.Microstructure.Status)
	}
	if r.Plan.State != agent2.StateActiveLimit {
		reasons = append(reasons, "plan not ACTIVE_LIMIT: "+string(r.Plan.State))
	}
	if r.Plan.ActionPermission != agent1.Allowed {
		reasons = append(reasons, "permission not ALLOWED: "+string(r.Plan.ActionPermission))
	}
	if r.Analysis.BTCAccumulation.Phase != accumulation.PhaseConfirmed {
		reasons = append(reasons, "BTC accumulation not ACCUMULATION_CONFIRMED: "+string(r.Analysis.BTCAccumulation.Phase))
	}
	if r.Proof.Status != liveguard.ReadyForManualLiveProofOrder {
		reasons = append(reasons, "proof not ready: "+r.Proof.Status)
	}
	if len(r.Desired) == 0 {
		reasons = append(reasons, "desired orders = 0")
	}
	if !r.CurrentDryRunApproved {
		reasons = append(reasons, "current plan dry-run not approved")
	}
	for _, item := range r.FinalAssertions {
		if strings.Contains(item, "assertion=BLOCK") {
			reasons = append(reasons, "final execution assertion block")
			break
		}
	}
	if !r.ForcedSimulation.Passed {
		reasons = append(reasons, "forced ACTIVE_LIMIT simulation failed")
	}
	return uniqueStringsMain(reasons)
}

func loadFirstOrderDryRunApproval(path string, now time.Time) (bool, []string) {
	reasons := []string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, []string{"approved dry-run audit unavailable: " + err.Error()}
	}
	var report liveAutoAuditReport
	if err := json.Unmarshal(b, &report); err != nil {
		return false, []string{"approved dry-run audit unreadable: " + err.Error()}
	}
	if report.GeneratedAt.IsZero() {
		reasons = append(reasons, "approved dry-run audit missing generated_at")
	} else if now.Sub(report.GeneratedAt) > 24*time.Hour || report.GeneratedAt.After(now.Add(5*time.Minute)) {
		reasons = append(reasons, "approved dry-run audit stale")
	}
	if report.Verdict != AuditApprovedDryRun && report.Verdict != AuditApprovedRealOrder {
		reasons = append(reasons, "approved dry-run audit verdict not approved: "+report.Verdict)
	}
	if !report.CurrentDryRunApproved {
		reasons = append(reasons, "current dry-run audit not approved")
	}
	if !report.ForcedSimulation.Passed {
		reasons = append(reasons, "forced simulation not passed")
	}
	return len(reasons) == 0, reasons
}

func liveAutoAuditMarkdown(r liveAutoAuditReport) string {
	var b strings.Builder
	b.WriteString("LIVE AUTO AUDIT\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", r.GeneratedAt.Format(time.RFC3339)))
	b.WriteString("Verdict: " + r.Verdict + "\n")
	b.WriteString(fmt.Sprintf("Monitoring: %v | Dry-run: %v | Real order: %v\n", r.Monitoring, r.DryRun, r.RealOrder))
	b.WriteString("CURRENT MARKET AUTHORITY: " + r.CurrentMarketAuthority + "\n")
	infra := "BLOCKED"
	if r.DryRunInfrastructureApproved {
		infra = "APPROVED"
	}
	b.WriteString("DRY-RUN INFRASTRUCTURE: " + infra + "\n")
	currentDryRun := "NOT_APPLICABLE"
	if len(r.CurrentDryRun.Desired) > 0 || len(r.CurrentDryRun.Placed) > 0 || len(r.CurrentDryRun.Blocked) > 0 {
		currentDryRun = "FAIL"
		if r.CurrentDryRunApproved {
			currentDryRun = "PASS"
		}
	}
	b.WriteString("CURRENT PLAN DRY-RUN: " + currentDryRun + "\n")
	b.WriteString("Summary: " + r.Summary + "\n\n")
	b.WriteString(fmt.Sprintf("Doctor: %s | %s\n", r.Doctor.Status, r.Doctor.Summary))
	b.WriteString(fmt.Sprintf("BTC: permission=%s accumulation=%s regime=%s trend=%.1f\n", r.Analysis.ActionPermission, r.Analysis.BTCAccumulation.Phase, r.Analysis.MarketRegime, r.Analysis.TrendScore))
	b.WriteString(fmt.Sprintf("Plan: %s | action_permission=%s | desired=%d blocked=%d\n", r.Plan.State, r.Plan.ActionPermission, len(r.Desired), len(r.Blocked)))
	b.WriteString(fmt.Sprintf("Microstructure: %s fresh=%d/%d blockers=%d\n", r.Microstructure.Status, r.Microstructure.FreshSymbols, r.Microstructure.RequiredFresh, len(r.Microstructure.Blockers)))
	b.WriteString(fmt.Sprintf("Proof: %s | %s\n", r.Proof.Status, r.Proof.Summary))
	b.WriteString(fmt.Sprintf("FORCED SYNTHETIC SIMULATION: %s | desired=%d would_place=%d blocked=%d exchange_calls=%d\n", r.ForcedSimulation.Status, r.ForcedSimulation.Desired, r.ForcedSimulation.WouldPlace, r.ForcedSimulation.Blocked, r.ForcedSimulation.ExchangeCalls))
	realOrder := "NOT_APPROVED"
	if r.RealOrder {
		realOrder = "APPROVED"
	}
	b.WriteString("REAL ORDER: " + realOrder + "\n")
	if len(r.FinalAssertions) > 0 {
		b.WriteString("Final assertion audit:\n")
		for _, item := range r.FinalAssertions {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(r.Reasons) > 0 {
		b.WriteString("\nReasons:\n")
		for _, reason := range r.Reasons {
			b.WriteString("- " + reason + "\n")
		}
	}
	b.WriteString("\nNo order was placed.\n")
	return b.String()
}
