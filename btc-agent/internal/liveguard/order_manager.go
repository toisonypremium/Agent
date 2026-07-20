package liveguard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

const managedExchangeTimeout = 10 * time.Second

const (
	ManagedCycleCompleted = "MANAGED_CYCLE_COMPLETED"
	ManagedCycleBlocked   = "MANAGED_CYCLE_BLOCKED"
	ManagedCyclePartial   = "MANAGED_CYCLE_PARTIAL"
	ManagedCycleDryRun    = "MANAGED_CYCLE_DRY_RUN"
)

type OrderCanceler interface {
	CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error)
}

type ManagedOrderRecorder interface {
	ReserveManagedLiveOrder(clientOrderID string, desired ManagedDesiredOrder, reason string) error
	MarkManagedLiveOrderSubmitted(clientOrderID string, result live.OrderResult) error
	MarkManagedLiveOrderRejected(clientOrderID string, reason string) error
	MarkManagedLiveOrderUnknown(clientOrderID string, reason string) error
}

type ThesisManagedOrderRecorder interface {
	ManagedOrderRecorder
	ReserveManagedLiveOrderWithThesis(clientOrderID string, desired ManagedDesiredOrder, reason string) error
}

func reserveManagedOrder(recorder ManagedOrderRecorder, clientOrderID string, desired ManagedDesiredOrder, reason string) error {
	if recorder == nil {
		return fmt.Errorf("managed order recorder unavailable")
	}
	if strings.TrimSpace(desired.ThesisID) != "" && strings.EqualFold(desired.Side, "BUY") {
		thesisRecorder, ok := recorder.(ThesisManagedOrderRecorder)
		if !ok {
			return fmt.Errorf("thesis-aware managed order recorder required")
		}
		return thesisRecorder.ReserveManagedLiveOrderWithThesis(clientOrderID, desired, reason)
	}
	return recorder.ReserveManagedLiveOrder(clientOrderID, desired, reason)
}

type ManagedDesiredOrder struct {
	ThesisID          string      `json:"thesis_id,omitempty"`
	Symbol            string      `json:"symbol"`
	InstID            string      `json:"inst_id"`
	LayerIndex        int         `json:"layer_index"`
	Side              string      `json:"side"`
	Type              string      `json:"type"`
	Price             float64     `json:"price"`
	Quantity          float64     `json:"quantity"`
	Notional          float64     `json:"notional"`
	PostOnly          bool        `json:"post_only"`
	InvalidationPrice float64     `json:"invalidation_price"`
	DiscountZone      market.Zone `json:"discount_zone"`
	Source            string      `json:"source"`
	DecisionReason    string      `json:"decision_reason"`
	QualityScore      float64     `json:"quality_score,omitempty"`
	QualityGrade      string      `json:"quality_grade,omitempty"`
	AllocationScore   float64     `json:"allocation_score,omitempty"`
	AllocationTier    string      `json:"allocation_tier,omitempty"`
	AllocationReason  string      `json:"allocation_reason,omitempty"`
	TargetPrice       float64     `json:"target_price,omitempty"`
	RewardRisk        float64     `json:"reward_risk,omitempty"`
	ExpiresAt         time.Time   `json:"expires_at,omitempty"`
	LayerReason       string      `json:"layer_reason,omitempty"`
	DecisionID        string      `json:"decision_id,omitempty"`
	Intent            string      `json:"intent,omitempty"`
	StrategyVersion   string      `json:"strategy_version,omitempty"`
	ConfigHash        string      `json:"config_hash,omitempty"`
}

type ManagedOrderDecision struct {
	Action        string                 `json:"action"`
	Symbol        string                 `json:"symbol"`
	LayerIndex    int                    `json:"layer_index,omitempty"`
	Reason        string                 `json:"reason"`
	Order         live.OrderStatus       `json:"order,omitempty"`
	Desired       ManagedDesiredOrder    `json:"desired,omitempty"`
	CancelResult  live.CancelOrderResult `json:"cancel_result,omitempty"`
	PlaceResult   live.OrderResult       `json:"place_result,omitempty"`
	Error         string                 `json:"error,omitempty"`
	ReplacedOrder bool                   `json:"replaced_order,omitempty"`
	AuditTrail    []string               `json:"audit_trail,omitempty"`
}

type ManagedCycleResult struct {
	GeneratedAt     time.Time              `json:"generated_at"`
	Status          string                 `json:"status"`
	PlanState       agent2.State           `json:"plan_state"`
	Desired         []ManagedDesiredOrder  `json:"desired"`
	Kept            []ManagedOrderDecision `json:"kept"`
	Canceled        []ManagedOrderDecision `json:"canceled"`
	Replaced        []ManagedOrderDecision `json:"replaced"`
	Placed          []ManagedOrderDecision `json:"placed"`
	Blocked         []ManagedOrderDecision `json:"blocked"`
	PerCoin         []ManagedCoinSummary   `json:"per_coin,omitempty"`
	Reasons         []string               `json:"reasons,omitempty"`
	DataHealth      DataHealthResult       `json:"data_health,omitempty"`
	ReconcileSafety ReconcileSafetyResult  `json:"reconcile_safety,omitempty"`
	RiskGovernor    RiskGovernorResult     `json:"risk_governor,omitempty"`
	Summary         string                 `json:"summary"`
	DryRun          bool                   `json:"dry_run,omitempty"`
}

type ManagedCoinSummary struct {
	Symbol              string                   `json:"symbol"`
	State               agent2.State             `json:"state"`
	ReadinessScore      float64                  `json:"readiness_score,omitempty"`
	DesiredLayers       int                      `json:"desired_layers"`
	OpenOrders          int                      `json:"open_orders"`
	Kept                int                      `json:"kept"`
	Canceled            int                      `json:"canceled"`
	Replaced            int                      `json:"replaced"`
	Placed              int                      `json:"placed"`
	Blocked             int                      `json:"blocked"`
	PendingNotional     float64                  `json:"pending_notional"`
	Actions             []ManagedOrderDecision   `json:"actions,omitempty"`
	Reasons             []string                 `json:"reasons,omitempty"`
	WhyNoOrder          []string                 `json:"why_no_order,omitempty"`
	HardBlockers        []string                 `json:"hard_blockers,omitempty"`
	SoftBlockers        []string                 `json:"soft_blockers,omitempty"`
	NextTrigger         string                   `json:"next_trigger,omitempty"`
	FilterAttribution   agent2.FilterAttribution `json:"filter_attribution,omitempty"`
	TopFilterBlocker    string                   `json:"top_filter_blocker,omitempty"`
	TopFilterBlockerKey string                   `json:"top_filter_blocker_key,omitempty"`
}

func ManageLiveOrders(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader) ManagedCycleResult {
	return ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, ManagedExecutionContext{}, nil, false)
}

func ManageLiveOrdersDryRun(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, dryRun bool) ManagedCycleResult {
	return ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, ManagedExecutionContext{}, nil, dryRun)
}

func ManageLiveOrdersWithRecorder(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	return ManageLiveOrdersWithRecorderAndContext(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, ManagedExecutionContext{}, recorder, dryRun)
}

func ManageLiveOrdersWithRecorderAndContext(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, execCtx ManagedExecutionContext, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
	result := ManagedCycleResult{GeneratedAt: time.Now(), Status: ManagedCycleCompleted, PlanState: plan.State, Desired: []ManagedDesiredOrder{}, DryRun: dryRun}
	if halted, err := haltedState(haltReader); err != nil || halted {
		result.Reasons = append(result.Reasons, "operator halt active")
		if !dryRun {
			result.Status = ManagedCycleBlocked
			result.PerCoin = BuildManagedCoinSummaries(cfg, plan, openOrders, result)
			result.Summary = managedSummary(result)
			return result
		}
	}
	if !dryRun && (placer == nil || canceler == nil) {
		result.Status = ManagedCycleBlocked
		result.Reasons = append(result.Reasons, "order placer/canceler unavailable")
		result.PerCoin = BuildManagedCoinSummaries(cfg, plan, openOrders, result)
		result.Summary = managedSummary(result)
		return result
	}
	desired, blocked := BuildManagedDesiredOrdersWithContext(cfg, plan, filters, positions, openOrders, execCtx)
	result.Desired = desired
	result.Blocked = append(result.Blocked, blocked...)
	desiredByKey := map[string]ManagedDesiredOrder{}
	for _, d := range desired {
		desiredByKey[managedKey(d.Symbol, d.LayerIndex)] = d
	}
	openByKey := map[string]live.OrderStatus{}
	duplicateKeys := map[string]bool{}
	openOrderCount := 0
	openOrderCountBySymbol := map[string]int{}
	openNotionalTotal := 0.0
	openNotionalBySymbol := map[string]float64{}
	for _, order := range openOrders {
		if order.Symbol == "" {
			order.Symbol = live.InternalSymbol(order.InstID)
		}
		if order.Notional <= 0 && order.Price > 0 && order.Quantity > 0 {
			order.Notional = order.Price * order.Quantity
		}
		key := orderKey(order)
		if key != "" {
			if _, exists := openByKey[key]; exists {
				duplicateKeys[key] = true
			} else {
				openByKey[key] = order
			}
		}
		symbol := strings.ToUpper(order.Symbol)
		openOrderCount++
		openOrderCountBySymbol[symbol]++
		openNotionalTotal += order.Notional
		openNotionalBySymbol[symbol] += order.Notional
	}
	if len(duplicateKeys) > 0 {
		for key := range duplicateKeys {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Reason: "duplicate open order managed key requires reconciliation: " + key})
		}
		result.Status = ManagedCycleBlocked
		result.Reasons = append(result.Reasons, "duplicate open order managed key")
		result.PerCoin = BuildManagedCoinSummaries(cfg, plan, openOrders, result)
		result.Summary = managedSummary(result)
		return result
	}

	for _, order := range openOrders {
		key := orderKey(order)
		desiredOrder, found := desiredByKey[key]
		if !found {
			if fallback, ok := matchBySymbolPrice(order, desired); ok {
				desiredOrder = fallback
				found = true
				key = managedKey(desiredOrder.Symbol, desiredOrder.LayerIndex)
			}
		}
		if !found || shouldCancelOpenOrder(cfg, plan, order, desiredOrder) {
			decision := ManagedOrderDecision{Action: "cancel", Symbol: orderSymbol(order), LayerIndex: order.LayerIndex, Order: order, Reason: cancelReason(cfg, plan, order, found)}
			if dryRun {
				decision.Action = "would_cancel"
				if found && priceDriftExceeded(cfg, order.Price, desiredOrder.Price) {
					decision.Desired = desiredOrder
					decision.ReplacedOrder = true
					result.Replaced = append(result.Replaced, decision)
				} else {
					result.Canceled = append(result.Canceled, decision)
				}
				continue
			}
			cancelCtx, cancelDone := context.WithTimeout(ctx, managedExchangeTimeout)
			cancel, err := canceler.CancelOrder(cancelCtx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
			cancelDone()
			decision.CancelResult = cancel
			if err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				continue
			}
			result.Canceled = append(result.Canceled, decision)
			openOrderCount--
			openOrderCountBySymbol[strings.ToUpper(orderSymbol(order))]--
			openNotionalTotal -= order.Notional
			openNotionalBySymbol[strings.ToUpper(orderSymbol(order))] -= order.Notional
			delete(openByKey, key)
			if found && priceDriftExceeded(cfg, order.Price, desiredOrder.Price) {
				decision.Action = "replace"
				decision.Desired = desiredOrder
				decision.ReplacedOrder = true
				result.Replaced = append(result.Replaced, decision)
			}
			continue
		}
		result.Kept = append(result.Kept, ManagedOrderDecision{Action: "keep", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Order: order, Desired: desiredOrder, Reason: "order still matches active accumulation layer"})
	}

	for _, desiredOrder := range desired {
		key := managedKey(desiredOrder.Symbol, desiredOrder.LayerIndex)
		if _, exists := openByKey[key]; exists {
			continue
		}
		if openOrderCountBySymbol[strings.ToUpper(desiredOrder.Symbol)] >= normalizedMaxOpenLiveOrdersPerAsset(cfg) {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "per-asset open order limit reached"})
			continue
		}
		if openOrderCount >= normalizedMaxOpenLiveOrdersTotal(cfg) {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "total open order limit reached"})
			continue
		}
		if openNotionalBySymbol[desiredOrder.Symbol]+desiredOrder.Notional > normalizedMaxLiveNotionalPerAsset(cfg)+1e-9 {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "per-asset live notional cap reached"})
			continue
		}
		if openNotionalTotal+desiredOrder.Notional > normalizedMaxLiveNotionalTotal(cfg)+1e-9 {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "total live notional cap reached"})
			continue
		}
		decision := ManagedOrderDecision{Action: "place", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "missing active accumulation layer order"}
		assertionBlockers := AssertManagedExecutionAllowed(ExecutionAssertionInput{Config: cfg, Plan: plan, Desired: desiredOrder, OpenNotionalTotal: openNotionalTotal, OpenNotionalBySymbol: openNotionalBySymbol, DryRun: dryRun, ManagedExecutionContext: execCtx})
		decision.AuditTrail = FinalAssertionAuditWithContext(execCtx, plan, desiredOrder, assertionBlockers)
		if len(assertionBlockers) > 0 {
			decision.Action = "block"
			decision.Reason = "final execution assertion blocked: " + strings.Join(assertionBlockers, "; ")
			result.Blocked = append(result.Blocked, decision)
			result.Status = ManagedCyclePartial
			continue
		}
		mmCtx, mmDone := context.WithTimeout(ctx, managedExchangeTimeout)
		mmGate := EvaluateMMExecutionGate(mmCtx, cfg, desiredOrder, orderBookProviderFromPlacer(placer))
		mmDone()
		if !mmGate.Pass {
			decision.Action = "block"
			decision.Reason = "MM execution gate blocked: " + strings.Join(mmGate.Reasons, "; ")
			result.Blocked = append(result.Blocked, decision)
			continue
		}
		if dryRun {
			decision.Action = "would_place"
			result.Placed = append(result.Placed, decision)
			openByKey[key] = live.OrderStatus{Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Notional: desiredOrder.Notional}
			openNotionalBySymbol[desiredOrder.Symbol] += desiredOrder.Notional
			openNotionalTotal += desiredOrder.Notional
			continue
		}
		desiredOrder = WithStrategyEvidence(cfg, desiredOrder)
		decision.Desired = desiredOrder
		clientID := clientOrderID(desiredOrder.Symbol)
		if recorder != nil {
			if err := reserveManagedOrder(recorder, clientID, desiredOrder, decision.Reason); err != nil {
				decision.Error = err.Error()
				decision.Reason = "reserve live order failed"
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				continue
			}
		}
		req := live.LimitOrderRequest{InstID: desiredOrder.InstID, Side: strings.ToLower(desiredOrder.Side), Price: desiredOrder.Price, Quantity: desiredOrder.Quantity, PostOnly: desiredOrder.PostOnly, ClientOrderID: clientID}
		placeCtx, placeDone := context.WithTimeout(ctx, managedExchangeTimeout)
		placed, err := placer.PlaceSpotLimitOrder(placeCtx, req)
		placeDone()
		decision.PlaceResult = placed
		if err != nil {
			safeErr := sanitizeExchangeError(cfg, err)
			unknown := managedSubmissionOutcomeUnknown(err)
			if recorder != nil {
				var persistErr error
				if unknown {
					persistErr = recorder.MarkManagedLiveOrderUnknown(clientID, safeErr)
				} else {
					persistErr = recorder.MarkManagedLiveOrderRejected(clientID, safeErr)
				}
				if persistErr != nil {
					decision.Error = safeErr + "; persist outcome failed: " + persistErr.Error()
					decision.Reason = "order outcome persistence failed; manual check required"
					result.Blocked = append(result.Blocked, decision)
					result.Status = ManagedCyclePartial
					continue
				}
			}
			decision.Error = safeErr
			if unknown {
				decision.Reason = "order placement outcome unknown; reconcile required"
			}
			result.Blocked = append(result.Blocked, decision)
			result.Status = ManagedCyclePartial
			continue
		}
		if recorder != nil {
			if err := recorder.MarkManagedLiveOrderSubmitted(clientID, placed); err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				result.Reasons = append(result.Reasons, "submitted order persistence failed")
				result.PerCoin = BuildManagedCoinSummaries(cfg, plan, openOrders, result)
				result.Summary = managedSummary(result)
				return result
			}
		}
		result.Placed = append(result.Placed, decision)
		openByKey[key] = live.OrderStatus{Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Notional: desiredOrder.Notional, ClientOrderID: clientID, OrderID: placed.OrderID, Status: live.StatusSubmitted}
		openNotionalBySymbol[desiredOrder.Symbol] += desiredOrder.Notional
		openNotionalTotal += desiredOrder.Notional
	}
	if dryRun {
		result.Status = ManagedCycleDryRun
	} else if result.Status != ManagedCyclePartial && len(result.Reasons) == 0 {
		result.Status = ManagedCycleCompleted
	}
	result.PerCoin = BuildManagedCoinSummaries(cfg, plan, openOrders, result)
	result.Summary = managedSummary(result)
	return result
}
