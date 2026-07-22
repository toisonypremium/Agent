package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

func executeLatestHermesDecision(ctx context.Context, cfg config.Config, db *storage.DB, plan agent2.Plan, analysis agent1.MarketAnalysis, open []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, dataHealth liveguard.DataHealthResult, reconcile liveguard.ReconcileSafetyResult, risk liveguard.RiskGovernorResult, placer liveguard.OrderPlacer, execCtx liveguard.ManagedExecutionContext, dryRun bool) (liveguard.ManagedCycleResult, bool) {
	if demoted, err := db.IsHermesDemoted(); err != nil && !dryRun {
		return blockedHermesCycle(plan, dryRun, "Hermes circuit-breaker state unavailable"), true
	} else if demoted && !dryRun {
		return blockedHermesCycle(plan, dryRun, "Hermes circuit-breaker demoted; human resume required"), true
	}
	if !cfg.HermesOperator.CanExecute() && !dryRun {
		return liveguard.ManagedCycleResult{}, false
	}
	b, err := os.ReadFile(filepath.Join("reports", "hermes_shadow_decision_latest.json"))
	if err != nil {
		return blockedHermesCycle(plan, dryRun, "Hermes decision audit unavailable"), true
	}
	var audit hermesShadowDecision
	if json.Unmarshal(b, &audit) != nil {
		return blockedHermesCycle(plan, dryRun, "Hermes decision audit invalid"), true
	}
	if audit.Mode != cfg.HermesOperator.NormalizedMode() {
		return blockedHermesCycle(plan, dryRun, "Hermes decision mode mismatch"), true
	}
	if time.Since(audit.GeneratedAt) > time.Duration(cfg.HermesOperator.DecisionTTLSeconds)*time.Second || time.Now().Before(audit.GeneratedAt) {
		return blockedHermesCycle(plan, dryRun, "Hermes decision audit stale"), true
	}
	if len(audit.Validation.Reasons) > 0 {
		return blockedHermesCycle(plan, dryRun, "Hermes decision did not pass validation"), true
	}
	if !dryRun && cfg.HermesOperator.NormalizedMode() == "canary" && hermesActionsIncreaseExposure(audit.Validation.Actions) {
		if len(open) != 0 {
			return blockedHermesCycle(plan, dryRun, fmt.Sprintf("Hermes canary requires zero current open orders; found %d", len(open))), true
		}
		owned, ownedErr := db.HermesOwnedPositions()
		if ownedErr != nil {
			return blockedHermesCycle(plan, dryRun, "Hermes canary owned-position state unavailable"), true
		}
		if len(owned) != 0 {
			return blockedHermesCycle(plan, dryRun, fmt.Sprintf("Hermes canary requires zero current owned positions; found %d", len(owned))), true
		}
		readiness, readinessErr := loadHermesCanaryReadiness(filepath.Join("reports", "hermes_canary_readiness_latest.json"))
		if readinessErr != nil {
			return blockedHermesCycle(plan, dryRun, "Hermes canary readiness unavailable: "+readinessErr.Error()), true
		}
		if blockers := liveguard.ValidateCanaryReadinessReport(readiness, time.Now().UTC()); len(blockers) > 0 {
			return blockedHermesCycle(plan, dryRun, "Hermes canary readiness blocked: "+strings.Join(blockers, "; ")), true
		}
	}
	if len(audit.Validation.Actions) == 0 {
		return noActionHermesCycle(plan, dryRun, "valid Hermes HOLD/WATCH decision; no action requested"), true
	}
	halted, haltErr := db.IsHalted()
	openExposure := 0.0
	assetExposure := map[string]float64{}
	for _, o := range open {
		n := o.Notional
		if n <= 0 {
			n = o.Price * o.Quantity
		}
		openExposure += n
		assetExposure[strings.ToUpper(o.Symbol)] += n
	}
	for _, p := range positions {
		openExposure += p.CostBasis
		assetExposure[strings.ToUpper(p.Symbol)] += p.CostBasis
	}
	assetRemaining := map[string]float64{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		assetRemaining[strings.ToUpper(symbol)] = maxFloat(0, cfg.Live.MaxLiveNotionalPerAssetUSDT-assetExposure[strings.ToUpper(symbol)])
	}
	safety := liveguard.EvaluateHermesActions(audit.Validation.Actions, liveguard.HermesSafetyContext{OperatorHalted: halted || haltErr != nil, DataHealthy: dataHealth.Status != liveguard.DataHealthBlock, ReconcileClean: reconcile.Status != liveguard.ReconcileBlock, OKXReady: placer != nil, PanicSelling: analysis.MarketRegime == "PANIC_SELLING", PortfolioNotionalRemaining: maxFloat(0, cfg.HermesOperator.MaxPortfolioExposureUSDT-openExposure), AssetNotionalRemaining: assetRemaining})
	if cfg.HermesOperator.NormalizedMode() == "canary" && len(safety) > 1 {
		safety = safety[:1]
	}
	decisionID := audit.Validation.Decision.DecisionID
	if len(safety) == 1 && safety[0].Allowed && safety[0].Action.Intent == hermesoperator.IntentCancel {
		canceler, _ := placer.(liveguard.OrderCanceler)
		statusReader, _ := placer.(liveguard.OrderStatusReader)
		result := liveguard.ExecuteHermesCancelActions(ctx, cfg, decisionID, safety, open, canceler, statusReader, db, dryRun)
		result.PlanState = plan.State
		result.DataHealth, result.ReconcileSafety, result.RiskGovernor = dataHealth, reconcile, risk
		return result, true
	}
	if len(safety) == 1 && safety[0].Allowed && safety[0].Action.Intent == hermesoperator.IntentReduce {
		owned, err := db.HermesOwnedPositions()
		if err != nil {
			return blockedHermesCycle(plan, dryRun, "Hermes owned-position ledger unavailable"), true
		}
		result := liveguard.ExecuteHermesReduceActions(ctx, cfg, decisionID, safety, owned, filters, placer, db, dryRun)
		result.PlanState = plan.State
		result.DataHealth, result.ReconcileSafety, result.RiskGovernor = dataHealth, reconcile, risk
		return result, true
	}
	if len(safety) == 1 && safety[0].Allowed && safety[0].Action.Intent == hermesoperator.IntentExitLimit {
		owned, err := db.HermesOwnedPositions()
		if err != nil {
			return blockedHermesCycle(plan, dryRun, "Hermes owned-position ledger unavailable"), true
		}
		result := liveguard.ExecuteHermesExitLimitActions(ctx, cfg, decisionID, safety, owned, filters, placer, db, dryRun)
		result.PlanState = plan.State
		result.DataHealth, result.ReconcileSafety, result.RiskGovernor = dataHealth, reconcile, risk
		return result, true
	}
	for _, decision := range safety {
		if decision.Allowed && decision.Action.Intent.ReducesExposure() {
			return blockedHermesCycle(plan, dryRun, "Hermes reducing intent lifecycle not implemented: "+string(decision.Action.Intent)), true
		}
	}
	desired, blocked := liveguard.BuildHermesDesiredOrders(cfg, plan, decisionID, true, safety, filters)
	if len(desired) == 0 {
		result := blockedHermesCycle(plan, dryRun, "no Hermes action survived production safety envelope")
		result.Blocked = append(result.Blocked, blocked...)
		return result, true
	}
	execCtx.BTCAccumulationPhase = string(analysis.BTCAccumulation.Phase)
	execCtx.HermesMode = cfg.HermesOperator.NormalizedMode()
	result := liveguard.ExecuteHermesDesiredOrders(ctx, cfg, plan, desired, open, placer, db, execCtx, dryRun)
	result.DataHealth, result.ReconcileSafety, result.RiskGovernor = dataHealth, reconcile, risk
	result.Blocked = append(result.Blocked, blocked...)
	return result, true
}

func noActionHermesCycle(plan agent2.Plan, dryRun bool, reason string) liveguard.ManagedCycleResult {
	status := liveguard.ManagedCycleCompleted
	if dryRun {
		status = liveguard.ManagedCycleDryRun
	}
	return liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: status, PlanState: plan.State, DryRun: dryRun, Reasons: []string{reason}, Summary: "HERMES_NO_ACTION: " + reason}
}

func hermesActionsIncreaseExposure(actions []hermesoperator.Action) bool {
	for _, action := range actions {
		if action.Intent.IncreasesExposure() {
			return true
		}
	}
	return false
}

func loadHermesCanaryReadiness(path string) (liveguard.CanaryReadinessResult, error) {
	var report liveguard.CanaryReadinessResult
	b, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return report, err
	}
	return report, nil
}

func blockedHermesCycle(plan agent2.Plan, dryRun bool, reason string) liveguard.ManagedCycleResult {
	return liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleBlocked, PlanState: plan.State, DryRun: dryRun, Reasons: []string{reason}, Summary: "HERMES_BLOCK: " + reason}
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
var _ hermesoperator.Decision
