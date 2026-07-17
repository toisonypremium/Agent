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

func executeLatestHermesDecision(ctx context.Context, cfg config.Config, db *storage.DB, plan agent2.Plan, analysis agent1.MarketAnalysis, open []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, dataHealth liveguard.DataHealthResult, reconcile liveguard.ReconcileSafetyResult, risk liveguard.RiskGovernorResult, placer liveguard.OrderPlacer, dryRun bool) (liveguard.ManagedCycleResult, bool) {
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
		assetRemaining[strings.ToUpper(symbol)] = maxFloat(0, config.EffectiveLiveNotionalPerAsset(cfg)-assetExposure[strings.ToUpper(symbol)])
	}
	mmConfidence := 0.0
	if fp, ok := analysis.Microstructure.MMFootprint[strings.ToUpper(cfg.Data.Symbols.BTC)]; ok {
		mmConfidence = fp.FootprintScore
	}
	liquidityQuality := map[string]float64{}
	for _, asset := range plan.Assets {
		q := 0.5
		if asset.LiquidityQuality.Pass {
			q = 1.0
		}
		liquidityQuality[strings.ToUpper(asset.Symbol)] = q
	}
	safety := liveguard.EvaluateHermesActions(audit.Validation.Actions, liveguard.HermesSafetyContext{
		OperatorHalted: halted || haltErr != nil, DataHealthy: dataHealth.Status != liveguard.DataHealthBlock,
		ReconcileClean: reconcile.Status != liveguard.ReconcileBlock, OKXReady: placer != nil,
		PanicSelling:               analysis.MarketRegime == "PANIC_SELLING",
		PortfolioNotionalRemaining: maxFloat(0, config.EffectiveLiveNotionalTotal(cfg)-openExposure),
		AssetNotionalRemaining:     assetRemaining, Autonomous: cfg.HermesOperator.NormalizedMode() == "autonomous",
		TotalCapital: cfg.Portfolio.TotalCapital, AccumulationPhase: string(analysis.BTCAccumulation.Phase),
		MarketRegime: analysis.MarketRegime, TrendScore: analysis.TrendScore, MMConfidence: mmConfidence,
		DataQuality: analysis.BTCAccumulation.DataQuality, LiquidityQuality: liquidityQuality,
		PerOrderCap: config.EffectiveLiveNotionalPerOrder(cfg),
	})
	if cfg.HermesOperator.NormalizedMode() == "canary" && len(safety) > 1 {
		safety = safety[:1]
	}
	decisionID := audit.Validation.Decision.DecisionID
	// Enforce the staged lifecycle before building exchange orders. Risk-reducing
	// actions remain independent; only exposure increases must pass the gate.
	assetsBySymbol := map[string]agent2.AssetPlan{}
	for _, asset := range plan.Assets {
		assetsBySymbol[strings.ToUpper(asset.Symbol)] = asset
	}
	openBuy := map[string]bool{}
	for _, order := range open {
		if strings.EqualFold(order.Side, "BUY") && live.IsOpenStatus(order.Status) {
			openBuy[strings.ToUpper(order.Symbol)] = true
		}
	}
	lastExitAt, lastExitErr := db.HermesLastExitAtBySymbol()
	lossLookback := time.Duration(cfg.Risk.HermesLossLookbackHours) * time.Hour
	if lossLookback <= 0 {
		lossLookback = 7 * 24 * time.Hour
	}
	lossProtection, lossProtectionErr := db.HermesLossProtectionSnapshot(time.Now().Add(-lossLookback))
	lossLockUntil := time.Time{}
	if cfg.Risk.HermesMaxConsecutiveLosses > 0 && lossProtection.ConsecutiveLosses >= cfg.Risk.HermesMaxConsecutiveLosses {
		lossLockUntil = lossProtection.LastLossAt.Add(time.Duration(cfg.Risk.HermesLossLockMinutes) * time.Minute)
	}
	filteredSafety := make([]liveguard.HermesActionDecision, 0, len(safety))
	for _, decision := range safety {
		if decision.Allowed && decision.Action.Intent.IncreasesExposure() {
			if lastExitErr != nil {
				decision.Allowed = false
				decision.Reasons = append(decision.Reasons, "Hermes exit history unavailable")
			}
			if lossProtectionErr != nil {
				decision.Allowed = false
				decision.Reasons = append(decision.Reasons, "Hermes loss history unavailable")
			} else if !lossLockUntil.IsZero() && time.Now().Before(lossLockUntil) {
				decision.Allowed = false
				decision.Reasons = append(decision.Reasons, fmt.Sprintf("Hermes loss-streak protection active until %s", lossLockUntil.UTC().Format(time.RFC3339)))
			}
			asset := assetsBySymbol[strings.ToUpper(decision.Action.Symbol)]
			cap := config.EffectiveLiveNotionalPerAsset(cfg)
			lifecycle := liveguard.EvaluateHermesLifecycle(liveguard.HermesLifecycleContext{Action: decision.Action, Asset: asset, ExistingNotional: assetExposure[strings.ToUpper(decision.Action.Symbol)], AssetCap: cap, HasOpenBuy: openBuy[strings.ToUpper(decision.Action.Symbol)], Now: time.Now(), LastExitAt: lastExitAt[strings.ToUpper(decision.Action.Symbol)], CooldownAfterExit: time.Duration(cfg.Risk.HermesReentryCooldownMinutes) * time.Minute})
			if !lifecycle.Allowed {
				decision.Allowed = false
				decision.Reasons = append(decision.Reasons, lifecycle.Reasons...)
			}
		}
		filteredSafety = append(filteredSafety, decision)
	}
	safety = filteredSafety
	reducing := make([]liveguard.HermesActionDecision, 0)
	for _, decision := range safety {
		if decision.Allowed && decision.Action.Intent.ReducesExposure() {
			reducing = append(reducing, decision)
		}
	}
	if len(reducing) > 0 {
		result := executeHermesReducingBatch(ctx, cfg, db, plan, decisionID, reducing, open, positions, filters, placer, dataHealth, reconcile, risk, dryRun)
		return result, true
	}
	desired, blocked := liveguard.BuildHermesDesiredOrders(cfg, plan, decisionID, true, safety, filters)
	if len(desired) == 0 {
		result := blockedHermesCycle(plan, dryRun, "no Hermes action survived production safety envelope")
		result.Blocked = append(result.Blocked, blocked...)
		return result, true
	}
	execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: string(analysis.BTCAccumulation.Phase), HermesMode: cfg.HermesOperator.NormalizedMode()}
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

// executeHermesReducingBatch executes all validated risk-reducing actions in
// one cycle. Mixed decisions intentionally reduce first; exposure increases are
// deferred until the next fresh Hermes decision.
func executeHermesReducingBatch(ctx context.Context, cfg config.Config, db *storage.DB, plan agent2.Plan, decisionID string, decisions []liveguard.HermesActionDecision, open []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer liveguard.OrderPlacer, dataHealth liveguard.DataHealthResult, reconcile liveguard.ReconcileSafetyResult, risk liveguard.RiskGovernorResult, dryRun bool) liveguard.ManagedCycleResult {
	out := liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleCompleted, PlanState: plan.State, DryRun: dryRun}
	owned, ownedErr := db.HermesOwnedPositions()
	for _, decision := range decisions {
		var one liveguard.ManagedCycleResult
		switch decision.Action.Intent {
		case hermesoperator.IntentCancel:
			canceler, _ := placer.(liveguard.OrderCanceler)
			statusReader, _ := placer.(liveguard.OrderStatusReader)
			one = liveguard.ExecuteHermesCancelActions(ctx, cfg, decisionID, []liveguard.HermesActionDecision{decision}, open, canceler, statusReader, db, dryRun)
		case hermesoperator.IntentReduce:
			if ownedErr != nil {
				one = blockedHermesCycle(plan, dryRun, "Hermes owned-position ledger unavailable")
			} else {
				one = liveguard.ExecuteHermesReduceActions(ctx, cfg, decisionID, []liveguard.HermesActionDecision{decision}, owned, filters, placer, db, dryRun)
			}
		case hermesoperator.IntentExitLimit:
			if ownedErr != nil {
				one = blockedHermesCycle(plan, dryRun, "Hermes owned-position ledger unavailable")
			} else {
				one = liveguard.ExecuteHermesExitLimitActions(ctx, cfg, decisionID, []liveguard.HermesActionDecision{decision}, owned, filters, placer, db, dryRun)
			}
		}
		out.Desired = append(out.Desired, one.Desired...)
		out.Canceled = append(out.Canceled, one.Canceled...)
		out.Placed = append(out.Placed, one.Placed...)
		out.Blocked = append(out.Blocked, one.Blocked...)
		out.Reasons = append(out.Reasons, one.Reasons...)
	}
	if dryRun {
		out.Status = liveguard.ManagedCycleDryRun
	} else if len(out.Blocked) > 0 {
		out.Status = liveguard.ManagedCyclePartial
	}
	out.DataHealth, out.ReconcileSafety, out.RiskGovernor = dataHealth, reconcile, risk
	out.Summary = fmt.Sprintf("HERMES_REDUCE_BATCH: actions=%d canceled=%d placed=%d blocked=%d", len(decisions), len(out.Canceled), len(out.Placed), len(out.Blocked))
	return out
}
