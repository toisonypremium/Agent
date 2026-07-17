package liveguard

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

// ExecuteHermesDesiredOrders sends only pre-built, safety-validated Hermes BUY
// limit orders through the existing recorder and exchange interfaces.
func ExecuteHermesDesiredOrders(ctx context.Context, cfg config.Config, plan agent2.Plan, desired []ManagedDesiredOrder, openOrders []live.OrderStatus, placer OrderPlacer, recorder ManagedOrderRecorder, execCtx ManagedExecutionContext, dryRun bool) ManagedCycleResult {
	result := ManagedCycleResult{Status: ManagedCycleCompleted, PlanState: plan.State, Desired: desired, DryRun: dryRun}
	if !cfg.HermesOperator.CanExecute() && !dryRun {
		result.Status = ManagedCycleBlocked
		result.Reasons = append(result.Reasons, "Hermes operator has no production execution authority")
		return result
	}
	openTotal, openBySymbol := openNotionalMaps(openOrders)
	for _, d := range desired {
		decision := ManagedOrderDecision{Action: "place", Symbol: d.Symbol, LayerIndex: d.LayerIndex, Desired: d, Reason: "Hermes validated operator action"}
		if d.Source != "HERMES_OPERATOR" || d.Intent == "" || d.DecisionID == "" {
			decision.Action = "block"
			decision.Reason = "invalid Hermes production provenance"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		ctxForOrder := execCtx
		ctxForOrder.HermesDecisionID, ctxForOrder.HermesIntent = d.DecisionID, d.Intent
		blockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: d, OpenNotionalTotal: openTotal, OpenNotionalBySymbol: openBySymbol, DryRun: dryRun, ManagedExecutionContext: ctxForOrder})
		decision.AuditTrail = FinalAssertionAuditWithContext(ctxForOrder, plan, d, blockers)
		if len(blockers) > 0 {
			decision.Action = "block"
			decision.Reason = "final execution assertion blocked: " + strings.Join(blockers, "; ")
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if dryRun {
			decision.Action = "would_place"
			result.Placed = append(result.Placed, decision)
			continue
		}
		if placer == nil || recorder == nil {
			decision.Action = "block"
			decision.Reason = "placer/recorder unavailable"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		d = WithStrategyEvidence(cfg, d)
		decision.Desired = d
		clientID := hermesClientOrderID(d)
		if err := recorder.ReserveManagedLiveOrder(clientID, d, decision.Reason); err != nil {
			decision.Action = "block"
			decision.Error = err.Error()
			decision.Reason = "duplicate or reserve failed"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		placed, err := placer.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: d.InstID, Side: "buy", Price: d.Price, Quantity: d.Quantity, PostOnly: true, ClientOrderID: clientID})
		decision.PlaceResult = placed
		if err != nil {
			safeErr := sanitizeExchangeError(cfg, err)
			_ = recorder.MarkManagedLiveOrderRejected(clientID, safeErr)
			decision.Action = "block"
			decision.Error = safeErr
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if err := recorder.MarkManagedLiveOrderSubmitted(clientID, placed); err != nil {
			decision.Action = "block"
			decision.Error = err.Error()
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		result.Placed = append(result.Placed, decision)
		openTotal += d.Notional
		openBySymbol[strings.ToUpper(d.Symbol)] += d.Notional
	}
	if dryRun {
		result.Status = ManagedCycleDryRun
	} else if len(result.Blocked) > 0 {
		result.Status = ManagedCyclePartial
	}
	result.Summary = fmt.Sprintf("%s: desired=%d placed=%d blocked=%d", result.Status, len(result.Desired), len(result.Placed), len(result.Blocked))
	return result
}

func hermesClientOrderID(d ManagedDesiredOrder) string {
	id := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(d.DecisionID, "-", ""), "_", ""))
	if len(id) > 12 {
		id = id[:12]
	}
	symbol := strings.ToLower(strings.TrimSuffix(strings.ReplaceAll(d.Symbol, "-", ""), "usdt"))
	return fmt.Sprintf("h%s%s%02d", id, symbol, d.LayerIndex)
}

// HermesCancelRecorder persists a confirmed cancellation and its audit event.
type HermesCancelRecorder interface {
	SaveLiveOrderStatus(live.OrderStatus) error
	SaveLiveOrderEvent(live.OrderStatus) error
}

// ExecuteHermesCancelActions cancels only an unambiguous, open order owned by
// HERMES_OPERATOR. Exchange or status-read errors are unknown outcomes: the
// local order remains open so the next reconcile must resolve it. A cancellation
// that raced with a fill is persisted as PARTIAL_FILL, forcing ledger reconcile
// before the order can become terminal locally.
func ExecuteHermesCancelActions(ctx context.Context, cfg config.Config, decisionID string, decisions []HermesActionDecision, openOrders []live.OrderStatus, canceler OrderCanceler, statusReader OrderStatusReader, recorder HermesCancelRecorder, dryRun bool) ManagedCycleResult {
	result := ManagedCycleResult{Status: ManagedCycleCompleted, DryRun: dryRun}
	if !cfg.HermesOperator.CanExecute() && !dryRun {
		result.Status = ManagedCycleBlocked
		result.Reasons = append(result.Reasons, "Hermes operator has no production execution authority")
		return result
	}
	for _, d := range decisions {
		if !d.Allowed || d.Action.Intent != "CANCEL" {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(d.Action.Symbol))
		matches := []live.OrderStatus{}
		for _, order := range openOrders {
			orderSymbol := strings.ToUpper(firstNonEmptyString(order.Symbol, live.InternalSymbol(order.InstID)))
			owned := order.Source == "HERMES_OPERATOR" && strings.HasPrefix(strings.ToLower(order.ClientOrderID), "h")
			if owned && orderSymbol == symbol && live.IsOpenStatus(order.Status) {
				matches = append(matches, order)
			}
		}
		decision := ManagedOrderDecision{Action: "cancel", Symbol: symbol, Reason: "Hermes validated owned-order cancellation"}
		if decisionID == "" {
			decision.Action, decision.Reason = "block", "Hermes cancel decision_id required"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if len(matches) != 1 {
			decision.Action = "block"
			decision.Reason = fmt.Sprintf("Hermes cancel requires exactly one owned open order; found %d", len(matches))
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		order := matches[0]
		decision.Order = order
		decision.AuditTrail = []string{"decision_id=" + decisionID, "intent=CANCEL", "ownership=HERMES_OPERATOR", "client_order_id_prefix=h", "match_count=1"}
		if dryRun {
			decision.Action = "would_cancel"
			result.Canceled = append(result.Canceled, decision)
			continue
		}
		if canceler == nil || statusReader == nil || recorder == nil {
			decision.Action, decision.Reason = "block", "canceler/status-reader/recorder unavailable"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		cancel, err := canceler.CancelOrder(ctx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
		decision.CancelResult = cancel
		if err != nil || !cancel.Canceled {
			decision.Action = "block"
			decision.Reason = "cancel outcome unknown; reconcile required"
			if err != nil {
				decision.Error = sanitizeExchangeError(cfg, err)
			}
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		remote, err := statusReader.OrderStatus(ctx, order.InstID, order.OrderID, order.ClientOrderID)
		if err != nil {
			decision.Action = "block"
			decision.Reason = "post-cancel status unknown; reconcile required"
			decision.Error = sanitizeExchangeError(cfg, err)
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		remote = withLocalOrderIdentity(remote, order)
		remote.Symbol = firstNonEmptyString(remote.Symbol, order.Symbol)
		remote.Source = order.Source
		remote.LastManagementAction = "Hermes CANCEL decision=" + decisionID
		filled := remote.AccumulatedFillSz
		if filled <= 0 {
			filled = remote.FilledQuantity
		}
		if filled > 0 {
			remote.Status = live.StatusPartialFill
			decision.Reason = "cancel confirmed with fill; ledger reconcile required"
			decision.AuditTrail = append(decision.AuditTrail, "post_cancel_fill=true", "local_status=PARTIAL_FILL")
		} else if live.NormalizeOrderStatus(remote.Status) != live.StatusCancelled {
			decision.Action = "block"
			decision.Reason = "post-cancel status not terminal; reconcile required"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		remote.UpdatedAt = time.Now().Unix()
		if err := recorder.SaveLiveOrderStatus(remote); err != nil {
			decision.Action, decision.Error = "block", err.Error()
			decision.Reason = "persist confirmed cancellation failed"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if err := recorder.SaveLiveOrderEvent(remote); err != nil {
			decision.Action, decision.Error = "block", err.Error()
			decision.Reason = "persist cancellation event failed"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		decision.Order = remote
		result.Canceled = append(result.Canceled, decision)
	}
	if dryRun {
		result.Status = ManagedCycleDryRun
	} else if len(result.Blocked) > 0 {
		result.Status = ManagedCyclePartial
	}
	result.Summary = fmt.Sprintf("HERMES_CANCEL: canceled=%d blocked=%d dry_run=%t", len(result.Canceled), len(result.Blocked), dryRun)
	return result
}

// ExecuteHermesReduceActions places only SELL limit orders backed by a positive
// HERMES_OPERATOR-owned ledger position. Requested quantity is capped at owned
// quantity and floored to the exchange step size, so this path cannot short.
func ExecuteHermesReduceActions(ctx context.Context, cfg config.Config, decisionID string, decisions []HermesActionDecision, ownedPositions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	return ExecuteHermesReduceActionsWithOpen(ctx, cfg, decisionID, decisions, ownedPositions, nil, filters, placer, recorder, dryRun)
}

// ExecuteHermesReduceActionsWithOpen reserves only uncommitted inventory. A
// partially-filled or live SELL already owns its remaining quantity; subtracting
// that residual prevents overlapping exits from collectively exceeding ownership.
func ExecuteHermesReduceActionsWithOpen(ctx context.Context, cfg config.Config, decisionID string, decisions []HermesActionDecision, ownedPositions []live.LivePosition, openOrders []live.OrderStatus, filters []live.InstrumentFilter, placer OrderPlacer, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	return executeHermesOwnedSellActions(ctx, cfg, decisionID, "REDUCE", decisions, ownedPositions, openOrders, filters, placer, recorder, dryRun)
}

// ExecuteHermesExitLimitActions exits an owned position using the same
// no-short, filter-normalized, restart-recoverable lifecycle as REDUCE.
func ExecuteHermesExitLimitActions(ctx context.Context, cfg config.Config, decisionID string, decisions []HermesActionDecision, ownedPositions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	return ExecuteHermesExitLimitActionsWithOpen(ctx, cfg, decisionID, decisions, ownedPositions, nil, filters, placer, recorder, dryRun)
}

// ExecuteHermesExitLimitActionsWithOpen applies the same residual reservation
// invariant as REDUCE. Existing SELL quantity remains reserved until exchange
// reconciliation makes that order terminal.
func ExecuteHermesExitLimitActionsWithOpen(ctx context.Context, cfg config.Config, decisionID string, decisions []HermesActionDecision, ownedPositions []live.LivePosition, openOrders []live.OrderStatus, filters []live.InstrumentFilter, placer OrderPlacer, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	return executeHermesOwnedSellActions(ctx, cfg, decisionID, "EXIT_LIMIT", decisions, ownedPositions, openOrders, filters, placer, recorder, dryRun)
}

func openSellResidualQuantity(symbol string, openOrders []live.OrderStatus) float64 {
	reserved := 0.0
	for _, order := range openOrders {
		orderSymbol := strings.ToUpper(firstNonEmptyString(order.Symbol, live.InternalSymbol(order.InstID)))
		if orderSymbol != symbol || !strings.EqualFold(order.Side, "SELL") || !live.IsOpenStatus(order.Status) {
			continue
		}
		filled := math.Max(order.FilledQuantity, order.AccumulatedFillSz)
		remaining := order.Quantity - filled
		if remaining > 0 {
			reserved += remaining
		}
	}
	return reserved
}

func executeHermesOwnedSellActions(ctx context.Context, cfg config.Config, decisionID, intent string, decisions []HermesActionDecision, ownedPositions []live.LivePosition, openOrders []live.OrderStatus, filters []live.InstrumentFilter, placer OrderPlacer, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	result := ManagedCycleResult{Status: ManagedCycleCompleted, DryRun: dryRun}
	if !cfg.HermesOperator.CanExecute() && !dryRun {
		result.Status = ManagedCycleBlocked
		result.Reasons = append(result.Reasons, "Hermes operator has no production execution authority")
		return result
	}
	for _, d := range decisions {
		if !d.Allowed || string(d.Action.Intent) != intent {
			continue
		}
		action := d.Action
		symbol := strings.ToUpper(strings.TrimSpace(action.Symbol))
		decision := ManagedOrderDecision{Action: strings.ToLower(intent), Symbol: symbol, Reason: "Hermes validated owned-position " + strings.ToLower(intent)}
		matches := []live.LivePosition{}
		for _, pos := range ownedPositions {
			if strings.EqualFold(pos.Symbol, symbol) && pos.Quantity > 0 {
				matches = append(matches, pos)
			}
		}
		if decisionID == "" || len(matches) != 1 {
			decision.Action = "block"
			decision.Reason = fmt.Sprintf("Hermes %s requires decision_id and exactly one owned positive position; found %d", intent, len(matches))
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		pos := matches[0]
		reservedQty := openSellResidualQuantity(symbol, openOrders)
		availableQty := pos.Quantity - reservedQty
		if availableQty <= fillEpsilon {
			decision.Action = "block"
			decision.Reason = fmt.Sprintf("Hermes %s has no unreserved owned quantity; owned=%.12f reserved_sell=%.12f", intent, pos.Quantity, reservedQty)
			decision.AuditTrail = []string{fmt.Sprintf("owned_qty=%.12f", pos.Quantity), fmt.Sprintf("reserved_sell_qty=%.12f", reservedQty), "residual_tracker=retained"}
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		filter, ok := findFilter(symbol, filters)
		if !ok {
			decision.Action, decision.Reason = "block", "instrument filter not found"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		price := floorToStep(action.EntryPrice, filter.TickSize)
		qty := action.RequestedNotionalUSDT / action.EntryPrice
		if qty > availableQty {
			qty = availableQty
		}
		qty = floorToStep(qty, filter.StepSize)
		notional := price * qty
		if price <= 0 || qty <= 0 || qty > availableQty+fillEpsilon || (filter.MinSize > 0 && qty < filter.MinSize) || (filter.MinNotional > 0 && notional < filter.MinNotional) {
			decision.Action, decision.Reason = "block", intent+" quantity failed owned-position or instrument limits"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		desired := ManagedDesiredOrder{Symbol: symbol, InstID: firstNonEmptyString(pos.InstID, filter.InstID, live.OKXInstID(symbol)), Side: "SELL", Type: "limit", Price: price, Quantity: qty, Notional: notional, PostOnly: false, Source: "HERMES_OPERATOR", DecisionReason: strings.Join(action.ReasonCodes, ","), DecisionID: decisionID, Intent: intent, AllocationReason: "Hermes owned-position " + strings.ToLower(intent)}
		decision.Desired = desired
		decision.AuditTrail = []string{"decision_id=" + decisionID, "intent=" + intent, "ownership=HERMES_OPERATOR", fmt.Sprintf("owned_qty=%.12f", pos.Quantity), fmt.Sprintf("reserved_sell_qty=%.12f", reservedQty), fmt.Sprintf("available_qty=%.12f", availableQty), fmt.Sprintf("sell_qty=%.12f", qty), "no_short=true"}
		if dryRun {
			decision.Action = "would_" + strings.ToLower(intent)
			result.Placed = append(result.Placed, decision)
			continue
		}
		if placer == nil || recorder == nil {
			decision.Action, decision.Reason = "block", "placer/recorder unavailable"
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		desired = WithStrategyEvidence(cfg, desired)
		decision.Desired = desired
		clientID := hermesClientOrderID(desired)
		if err := recorder.ReserveManagedLiveOrder(clientID, desired, decision.Reason); err != nil {
			decision.Action, decision.Reason, decision.Error = "block", "duplicate or reserve failed", err.Error()
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		placed, err := placer.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: desired.InstID, Side: "sell", Price: price, Quantity: qty, PostOnly: false, ClientOrderID: clientID})
		decision.PlaceResult = placed
		if err != nil {
			// Submission may have reached the exchange. Keep PLANNED so startup
			// reconciliation resolves the client ID instead of claiming rejection.
			decision.Action, decision.Reason = "block", intent+" placement outcome unknown; reconcile required"
			decision.Error = sanitizeExchangeError(cfg, err)
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if err := recorder.MarkManagedLiveOrderSubmitted(clientID, placed); err != nil {
			decision.Action, decision.Reason, decision.Error = "block", "persist "+intent+" submission failed", err.Error()
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		result.Placed = append(result.Placed, decision)
	}
	if dryRun {
		result.Status = ManagedCycleDryRun
	} else if len(result.Blocked) > 0 {
		result.Status = ManagedCyclePartial
	}
	result.Summary = fmt.Sprintf("HERMES_%s: placed=%d blocked=%d dry_run=%t", intent, len(result.Placed), len(result.Blocked), dryRun)
	return result
}
