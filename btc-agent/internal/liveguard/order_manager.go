package liveguard

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

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
}

type ManagedDesiredOrder struct {
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
	return ManageLiveOrdersWithRecorder(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, nil, false)
}

func ManageLiveOrdersDryRun(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, dryRun bool) ManagedCycleResult {
	return ManageLiveOrdersWithRecorder(ctx, cfg, plan, openOrders, positions, filters, placer, canceler, haltReader, nil, dryRun)
}

func ManageLiveOrdersWithRecorder(ctx context.Context, cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, positions []live.LivePosition, filters []live.InstrumentFilter, placer OrderPlacer, canceler OrderCanceler, haltReader HaltReader, recorder ManagedOrderRecorder, dryRun bool) ManagedCycleResult {
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
	desired, blocked := BuildManagedDesiredOrders(cfg, plan, filters, positions, openOrders)
	result.Desired = desired
	result.Blocked = append(result.Blocked, blocked...)
	desiredByKey := map[string]ManagedDesiredOrder{}
	for _, d := range desired {
		desiredByKey[managedKey(d.Symbol, d.LayerIndex)] = d
	}
	openByKey := map[string]live.OrderStatus{}
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
			openByKey[key] = order
		}
		openNotionalTotal += order.Notional
		openNotionalBySymbol[strings.ToUpper(order.Symbol)] += order.Notional
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
			cancel, err := canceler.CancelOrder(ctx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
			decision.CancelResult = cancel
			if err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				continue
			}
			result.Canceled = append(result.Canceled, decision)
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
		if countOpenForSymbol(openByKey, desiredOrder.Symbol) >= normalizedMaxOpenLiveOrdersPerAsset(cfg) {
			result.Blocked = append(result.Blocked, ManagedOrderDecision{Action: "block", Symbol: desiredOrder.Symbol, LayerIndex: desiredOrder.LayerIndex, Desired: desiredOrder, Reason: "per-asset open order limit reached"})
			continue
		}
		if len(openByKey) >= normalizedMaxOpenLiveOrdersTotal(cfg) {
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
		mmGate := EvaluateMMExecutionGate(ctx, cfg, desiredOrder, orderBookProviderFromPlacer(placer))
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
		clientID := clientOrderID(desiredOrder.Symbol)
		if recorder != nil {
			if err := recorder.ReserveManagedLiveOrder(clientID, desiredOrder, decision.Reason); err != nil {
				decision.Error = err.Error()
				decision.Reason = "reserve live order failed"
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				continue
			}
		}
		req := live.LimitOrderRequest{InstID: desiredOrder.InstID, Side: strings.ToLower(desiredOrder.Side), Price: desiredOrder.Price, Quantity: desiredOrder.Quantity, PostOnly: desiredOrder.PostOnly, ClientOrderID: clientID}
		placed, err := placer.PlaceSpotLimitOrder(ctx, req)
		decision.PlaceResult = placed
		if err != nil {
			safeErr := sanitizeExchangeError(cfg, err)
			if recorder != nil {
				_ = recorder.MarkManagedLiveOrderRejected(clientID, safeErr)
			}
			decision.Error = safeErr
			result.Blocked = append(result.Blocked, decision)
			result.Status = ManagedCyclePartial
			continue
		}
		if recorder != nil {
			if err := recorder.MarkManagedLiveOrderSubmitted(clientID, placed); err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = ManagedCyclePartial
				continue
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

func BuildManagedCoinSummaries(cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, result ManagedCycleResult) []ManagedCoinSummary {
	symbols := []string{}
	seen := map[string]bool{}
	addSymbol := func(symbol string) {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || seen[symbol] {
			return
		}
		seen[symbol] = true
		symbols = append(symbols, symbol)
	}
	for _, symbol := range cfg.Data.Symbols.Assets {
		addSymbol(symbol)
	}
	stateBySymbol := map[string]agent2.State{}
	reasonBySymbol := map[string]string{}
	hardBySymbol := map[string][]string{}
	softBySymbol := map[string][]string{}
	nextBySymbol := map[string]string{}
	attributionBySymbol := map[string]agent2.FilterAttribution{}
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		addSymbol(symbol)
		stateBySymbol[symbol] = asset.State
		reasonBySymbol[symbol] = asset.Reason
		hardBySymbol[symbol] = appendUniqueStrings(hardBySymbol[symbol], asset.HardBlockers...)
		softBySymbol[symbol] = appendUniqueStrings(softBySymbol[symbol], asset.SoftBlockers...)
		nextBySymbol[symbol] = asset.NextTrigger
		attributionBySymbol[symbol] = agent2.BuildFilterAttribution(asset)
	}
	watchBySymbol := map[string]agent2.WatchCandidate{}
	for _, candidate := range plan.Watchlist.Candidates {
		symbol := strings.ToUpper(candidate.Symbol)
		addSymbol(symbol)
		watchBySymbol[symbol] = candidate
	}
	for _, order := range openOrders {
		addSymbol(orderSymbol(order))
	}
	for _, d := range result.Desired {
		addSymbol(d.Symbol)
	}
	for _, decision := range allManagedDecisions(result) {
		addSymbol(decisionSymbol(decision))
	}

	summaries := map[string]*ManagedCoinSummary{}
	for _, symbol := range symbols {
		state, ok := stateBySymbol[symbol]
		if !ok {
			state = defaultCoinState(plan.State)
		}
		summaries[symbol] = &ManagedCoinSummary{Symbol: symbol, State: state}
	}
	ensure := func(symbol string) *ManagedCoinSummary {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			symbol = "UNKNOWN"
		}
		if summaries[symbol] == nil {
			addSymbol(symbol)
			summaries[symbol] = &ManagedCoinSummary{Symbol: symbol, State: defaultCoinState(plan.State)}
		}
		return summaries[symbol]
	}
	for _, d := range result.Desired {
		ensure(d.Symbol).DesiredLayers++
	}
	for _, order := range openOrders {
		s := ensure(orderSymbol(order))
		s.OpenOrders++
		notional := order.Notional
		if notional <= 0 && order.Price > 0 && order.Quantity > 0 {
			notional = order.Price * order.Quantity
		}
		s.PendingNotional += notional
	}
	addDecision := func(decision ManagedOrderDecision, counter func(*ManagedCoinSummary)) {
		s := ensure(decisionSymbol(decision))
		counter(s)
		s.Actions = append(s.Actions, decision)
		if decision.Reason != "" && !stringInSlice(s.Reasons, decision.Reason) {
			s.Reasons = append(s.Reasons, decision.Reason)
			if decision.Action == "block" || decision.Error != "" {
				s.HardBlockers = appendUniqueStrings(s.HardBlockers, decision.Reason)
			} else {
				s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, decision.Reason)
			}
		}
	}
	for _, d := range result.Kept {
		addDecision(d, func(s *ManagedCoinSummary) { s.Kept++ })
	}
	for _, d := range result.Canceled {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Canceled++
			s.PendingNotional -= decisionOrderNotional(d)
		})
	}
	for _, d := range result.Replaced {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Replaced++
			s.PendingNotional -= decisionOrderNotional(d)
		})
	}
	for _, d := range result.Placed {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Placed++
			s.PendingNotional += d.Desired.Notional
		})
	}
	for _, d := range result.Blocked {
		addDecision(d, func(s *ManagedCoinSummary) { s.Blocked++ })
	}
	if len(result.Reasons) > 0 {
		for _, symbol := range symbols {
			s := summaries[symbol]
			if s == nil {
				continue
			}
			for _, reason := range result.Reasons {
				if reason != "" && !stringInSlice(s.Reasons, reason) {
					s.Reasons = append(s.Reasons, reason)
				}
			}
		}
	}
	for _, symbol := range symbols {
		s := summaries[symbol]
		if s == nil {
			continue
		}
		s.HardBlockers = appendUniqueStrings(s.HardBlockers, hardBySymbol[symbol]...)
		s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, softBySymbol[symbol]...)
		if nextBySymbol[symbol] != "" {
			s.NextTrigger = nextBySymbol[symbol]
		}
		if candidate, ok := watchBySymbol[symbol]; ok {
			s.ReadinessScore = candidate.ReadinessScore
			if s.NextTrigger == "" {
				s.NextTrigger = candidate.NextTrigger
			}
			if s.DesiredLayers == 0 && s.Placed == 0 && s.Kept == 0 {
				s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, candidate.Missing...)
			}
		}
		if s.DesiredLayers == 0 && s.Placed == 0 && s.Kept == 0 {
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.HardBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.SoftBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.Reasons...)
			if reason := reasonBySymbol[symbol]; reason != "" {
				s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, reason)
			}
			if len(s.WhyNoOrder) == 0 {
				s.WhyNoOrder = append(s.WhyNoOrder, "chưa có ACTIVE_LIMIT layer hợp lệ cho coin này")
			}
		}
		if attr, ok := attributionBySymbol[symbol]; ok {
			s.FilterAttribution = attr
			s.TopFilterBlocker = attr.TopBlocker
			s.TopFilterBlockerKey = attr.TopBlockerKey
		}
		if s.DesiredLayers > 0 && s.Placed == 0 && s.Kept == 0 && s.Blocked > 0 {
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.HardBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.Reasons...)
		}
		s.WhyNoOrder = agent2.CompactReasons(s.WhyNoOrder, 6)
		s.Reasons = agent2.CompactReasons(s.Reasons, 8)
	}
	order := map[string]int{}
	for i, symbol := range cfg.Data.Symbols.Assets {
		order[strings.ToUpper(symbol)] = i
	}
	sort.Slice(symbols, func(i, j int) bool {
		li, lok := order[symbols[i]]
		rj, rok := order[symbols[j]]
		if lok && rok {
			return li < rj
		}
		if lok != rok {
			return lok
		}
		return symbols[i] < symbols[j]
	})
	out := []ManagedCoinSummary{}
	for _, symbol := range symbols {
		if summaries[symbol] != nil {
			out = append(out, *summaries[symbol])
		}
	}
	return out
}

func allManagedDecisions(result ManagedCycleResult) []ManagedOrderDecision {
	out := []ManagedOrderDecision{}
	out = append(out, result.Kept...)
	out = append(out, result.Canceled...)
	out = append(out, result.Replaced...)
	out = append(out, result.Placed...)
	out = append(out, result.Blocked...)
	return out
}

func decisionSymbol(decision ManagedOrderDecision) string {
	if decision.Symbol != "" {
		return decision.Symbol
	}
	if decision.Desired.Symbol != "" {
		return decision.Desired.Symbol
	}
	return orderSymbol(decision.Order)
}

func decisionOrderNotional(decision ManagedOrderDecision) float64 {
	notional := decision.Order.Notional
	if notional <= 0 && decision.Order.Price > 0 && decision.Order.Quantity > 0 {
		notional = decision.Order.Price * decision.Order.Quantity
	}
	return notional
}

func defaultCoinState(planState agent2.State) agent2.State {
	if planState == agent2.StateWatch || planState == agent2.StateArmed || planState == agent2.StateNoTrade {
		return planState
	}
	return agent2.StateNoTrade
}

func stringInSlice(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func appendUniqueStrings(items []string, values ...string) []string {
	for _, value := range values {
		if value == "" || stringInSlice(items, value) {
			continue
		}
		items = append(items, value)
	}
	return items
}

func BuildManagedDesiredOrders(cfg config.Config, plan agent2.Plan, filters []live.InstrumentFilter, positions []live.LivePosition, openOrders []live.OrderStatus) ([]ManagedDesiredOrder, []ManagedOrderDecision) {
	desired := []ManagedDesiredOrder{}
	blocked := []ManagedOrderDecision{}
	if plan.State != agent2.StateActiveLimit || plan.ActionPermission != agent1.Allowed {
		return desired, blocked
	}
	qualityBySymbol := loadHistoryQualityScores("reports/live_manager_history_latest.json")
	allocationBySymbol := AllocateLiveCapital(cfg, plan, qualityBySymbol, positions, openOrders)
	totalDesired := 0.0
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		if asset.State != agent2.StateActiveLimit {
			continue
		}
		allocation := allocationBySymbol[symbol]
		if cfg.Live.LiquidityGateEnabled && asset.LiquidityQuality.Enabled && !asset.LiquidityQuality.Pass {
			reason := "liquidity gate blocked: " + liquidity.FirstReason(asset.LiquidityQuality.Reasons)
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: reason})
			continue
		}
		if allocation.Tier == OpportunityBlock || allocation.MaxLayers <= 0 || allocation.BudgetUSDT <= 0 {
			reason := "live allocation blocked: " + allocation.Reason
			if strings.EqualFold(allocation.QualityGrade, "D") {
				reason = "live quality filter blocked D-grade coin"
			}
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: reason})
			continue
		}
		assetRemaining := allocation.BudgetUSDT
		layers := asset.Layers
		if len(layers) > allocation.MaxLayers {
			layers = layers[:allocation.MaxLayers]
		}
		for _, layer := range layers {
			if assetRemaining <= 0 || totalDesired >= normalizedMaxLiveNotionalTotal(cfg) {
				break
			}
			notional := allocation.PerOrderUSDT
			if notional <= 0 {
				notional = normalizedMaxLiveNotionalPerOrder(cfg)
			}
			if cap := normalizedMaxLiveNotionalPerOrder(cfg); notional > cap {
				notional = cap
			}
			if notional > assetRemaining {
				notional = assetRemaining
			}
			if remainingTotal := normalizedMaxLiveNotionalTotal(cfg) - totalDesired; notional > remainingTotal {
				notional = remainingTotal
			}
			candidate, ok := candidateFromLayer(cfg, symbol, layer, notional)
			if !ok {
				blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, LayerIndex: layer.Index, Reason: "invalid layer candidate"})
				continue
			}
			preflightCandidate := candidate
			instID := live.OKXInstID(symbol)
			if len(filters) > 0 {
				var preflight PreflightResult
				preflightCandidate, preflight = RunPreflight(cfg, candidate, filters)
				if !preflight.Pass {
					blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, LayerIndex: layer.Index, Reason: "preflight not pass"})
					continue
				}
				instID = firstNonEmptyString(preflight.InstID, instID)
			}
			d := ManagedDesiredOrder{Symbol: symbol, InstID: instID, LayerIndex: layer.Index, Side: "BUY", Type: "limit", Price: preflightCandidate.Price, Quantity: preflightCandidate.Quantity, Notional: preflightCandidate.Notional, PostOnly: preflightCandidate.PostOnly, InvalidationPrice: asset.Invalidation, DiscountZone: asset.DiscountZone, Source: preflightCandidate.Source, DecisionReason: asset.Reason, QualityScore: qualityBySymbol[symbol].Score, QualityGrade: qualityBySymbol[symbol].Grade, AllocationScore: allocation.Score, AllocationTier: string(allocation.Tier), AllocationReason: allocation.Reason, TargetPrice: layer.Target, RewardRisk: layer.RewardRisk, ExpiresAt: layer.ExpiresAt, LayerReason: layer.Reason}
			desired = append(desired, d)
			assetRemaining -= d.Notional
			totalDesired += d.Notional
		}
	}
	sort.SliceStable(desired, func(i, j int) bool {
		if desired[i].AllocationScore != desired[j].AllocationScore {
			return desired[i].AllocationScore > desired[j].AllocationScore
		}
		if desired[i].QualityScore != desired[j].QualityScore {
			return desired[i].QualityScore > desired[j].QualityScore
		}
		if desired[i].Symbol == desired[j].Symbol {
			return desired[i].LayerIndex < desired[j].LayerIndex
		}
		return desired[i].Symbol < desired[j].Symbol
	})
	return desired, blocked
}

type historyQualityScore struct {
	Score float64
	Grade string
}

func loadHistoryQualityScores(path string) map[string]historyQualityScore {
	out := map[string]historyQualityScore{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var report struct {
		PerCoin map[string]struct {
			QualityScore float64 `json:"quality_score"`
			QualityGrade string  `json:"quality_grade"`
		} `json:"per_coin"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return out
	}
	for symbol, stats := range report.PerCoin {
		out[strings.ToUpper(symbol)] = historyQualityScore{Score: stats.QualityScore, Grade: stats.QualityGrade}
	}
	return out
}

func shouldCancelOpenOrder(cfg config.Config, plan agent2.Plan, order live.OrderStatus, desired ManagedDesiredOrder) bool {
	if cfg.Live.CancelIfPlanNotActive && plan.State != agent2.StateActiveLimit {
		return true
	}
	if desired.Symbol == "" {
		return true
	}
	if priceDriftExceeded(cfg, order.Price, desired.Price) {
		return true
	}
	if cfg.Live.CancelIfPriceAboveDiscountZonePct > 0 && order.Price > desired.DiscountZone.High*(1+cfg.Live.CancelIfPriceAboveDiscountZonePct) {
		return true
	}
	if cfg.Live.CancelStaleAfterMinutes > 0 && order.SubmittedAt > 0 && time.Since(time.Unix(order.SubmittedAt, 0)) > time.Duration(cfg.Live.CancelStaleAfterMinutes)*time.Minute {
		return true
	}
	return false
}

func cancelReason(cfg config.Config, plan agent2.Plan, order live.OrderStatus, foundDesired bool) string {
	if cfg.Live.CancelIfPlanNotActive && plan.State != agent2.StateActiveLimit {
		return "plan no longer ACTIVE_LIMIT"
	}
	if !foundDesired {
		return "order no longer matches active asset/layer"
	}
	return "order no longer matches current desired layer"
}

func priceDriftExceeded(cfg config.Config, current, desired float64) bool {
	if cfg.Live.ReplaceIfPriceDriftPct <= 0 || current <= 0 || desired <= 0 {
		return false
	}
	return math.Abs(current-desired)/desired > cfg.Live.ReplaceIfPriceDriftPct
}

func haltedState(h HaltReader) (bool, error) {
	if h == nil {
		return true, fmt.Errorf("halt reader unavailable")
	}
	return h.IsHalted()
}

func managedKey(symbol string, layer int) string {
	return strings.ToUpper(symbol) + "#" + fmt.Sprint(layer)
}

func orderKey(o live.OrderStatus) string {
	if o.LayerIndex <= 0 {
		return ""
	}
	return managedKey(orderSymbol(o), o.LayerIndex)
}

func orderSymbol(o live.OrderStatus) string {
	if o.Symbol != "" {
		return strings.ToUpper(o.Symbol)
	}
	return live.InternalSymbol(o.InstID)
}

func matchBySymbolPrice(order live.OrderStatus, desired []ManagedDesiredOrder) (ManagedDesiredOrder, bool) {
	for _, d := range desired {
		if d.Symbol == orderSymbol(order) && math.Abs(order.Price-d.Price) <= math.Max(1e-9, d.Price*0.0001) {
			return d, true
		}
	}
	return ManagedDesiredOrder{}, false
}

func countOpenForSymbol(open map[string]live.OrderStatus, symbol string) int {
	count := 0
	for _, o := range open {
		if orderSymbol(o) == strings.ToUpper(symbol) {
			count++
		}
	}
	return count
}

func managedSummary(r ManagedCycleResult) string {
	if len(r.Reasons) > 0 {
		return r.Status + ": " + strings.Join(r.Reasons, "; ")
	}
	return fmt.Sprintf("%s: desired=%d kept=%d canceled=%d replaced=%d placed=%d blocked=%d", r.Status, len(r.Desired), len(r.Kept), len(r.Canceled), len(r.Replaced), len(r.Placed), len(r.Blocked))
}

func normalizedMaxAutoLayersPerAsset(cfg config.Config) int {
	if cfg.Live.MaxAutoLayersPerAsset > 0 {
		return minInt(cfg.Live.MaxAutoLayersPerAsset, 3)
	}
	return normalizedMaxAutoLayers(cfg)
}

func normalizedMaxOpenLiveOrdersPerAsset(cfg config.Config) int {
	if cfg.Live.MaxOpenLiveOrdersPerAsset > 0 {
		return cfg.Live.MaxOpenLiveOrdersPerAsset
	}
	return normalizedMaxOpenLiveOrders(cfg)
}

func normalizedMaxOpenLiveOrdersTotal(cfg config.Config) int {
	if cfg.Live.MaxOpenLiveOrdersTotal > 0 {
		return cfg.Live.MaxOpenLiveOrdersTotal
	}
	return normalizedMaxOpenLiveOrdersPerAsset(cfg) * len(cfg.Data.Symbols.Assets)
}

func normalizedMaxLiveNotionalPerOrder(cfg config.Config) float64 {
	if cfg.Live.MaxLiveNotionalPerOrderUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerOrderUSDT
	}
	return normalizedAutoLadderMaxNotional(cfg)
}

func normalizedMaxLiveNotionalPerAsset(cfg config.Config) float64 {
	if cfg.Live.MaxLiveNotionalPerAssetUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerAssetUSDT
	}
	return normalizedMaxLiveNotionalPerOrder(cfg) * float64(normalizedMaxAutoLayersPerAsset(cfg))
}

func normalizedMaxLiveNotionalTotal(cfg config.Config) float64 {
	if cfg.Live.MaxLiveNotionalTotalUSDT > 0 {
		return cfg.Live.MaxLiveNotionalTotalUSDT
	}
	return normalizedMaxLiveNotionalPerAsset(cfg) * float64(len(cfg.Data.Symbols.Assets))
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
