package liveguard

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

const maxHistoryEvents = 300

type LiveManagerHistoryResult struct {
	GeneratedAt                       time.Time                          `json:"generated_at"`
	PeriodStart                       time.Time                          `json:"period_start"`
	PeriodEnd                         time.Time                          `json:"period_end"`
	WindowsTested                     int                                `json:"windows_tested"`
	Summary                           string                             `json:"summary"`
	ResearchArmed                     bool                               `json:"research_armed,omitempty"`
	ResearchProfile                   string                             `json:"research_profile,omitempty"`
	ResearchExpiryDays                int                                `json:"research_expiry_days,omitempty"`
	ResearchHoldWatch                 bool                               `json:"research_hold_through_watch,omitempty"`
	ResearchHoldPriceAboveDiscountPct float64                            `json:"research_hold_if_price_above_discount_pct,omitempty"`
	ProductionArmedProbe              bool                               `json:"production_armed_probe,omitempty"`
	ArmedProbe                        LiveManagerHistoryStats            `json:"armed_probe,omitempty"`
	Total                             LiveManagerHistoryStats            `json:"total"`
	PerCoin                           map[string]LiveManagerHistoryStats `json:"per_coin"`
	Events                            []LiveManagerHistoryEvent          `json:"events,omitempty"`
	Notes                             []string                           `json:"notes,omitempty"`
}

type LiveManagerHistoryOptions struct {
	ResearchArmed                     bool    `json:"research_armed,omitempty"`
	ResearchProfile                   string  `json:"research_profile,omitempty"`
	ResearchExpiryDays                int     `json:"research_expiry_days,omitempty"`
	ResearchHoldWatch                 bool    `json:"research_hold_through_watch,omitempty"`
	ResearchHoldPriceAboveDiscountPct float64 `json:"research_hold_if_price_above_discount_pct,omitempty"`
	ProductionArmedProbe              bool    `json:"production_armed_probe,omitempty"`
}

type LiveManagerHistoryStats struct {
	Desired       int            `json:"desired"`
	Placed        int            `json:"placed"`
	Kept          int            `json:"kept"`
	Canceled      int            `json:"canceled"`
	Replaced      int            `json:"replaced"`
	Blocked       int            `json:"blocked"`
	Filled        int            `json:"filled"`
	Expired       int            `json:"expired"`
	FillRate      float64        `json:"fill_rate"`
	CancelRate    float64        `json:"cancel_rate"`
	ReplaceRate   float64        `json:"replace_rate"`
	QualityScore  float64        `json:"quality_score"`
	QualityGrade  string         `json:"quality_grade,omitempty"`
	QualityReason string         `json:"quality_reason,omitempty"`
	BestLayer     int            `json:"best_layer,omitempty"`
	LayerFills    map[int]int    `json:"layer_fills,omitempty"`
	Blockers      map[string]int `json:"blockers,omitempty"`
	CancelReasons map[string]int `json:"cancel_reasons,omitempty"`
	DesiredLoss   map[string]int `json:"desired_loss,omitempty"`
}

type LiveManagerHistoryEvent struct {
	Time     string  `json:"time"`
	Symbol   string  `json:"symbol"`
	Type     string  `json:"type"`
	Layer    int     `json:"layer,omitempty"`
	Price    float64 `json:"price,omitempty"`
	Notional float64 `json:"notional,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

type historyOpenOrder struct {
	Status          live.OrderStatus
	PlacedIndex     int
	ActiveFromIndex int
}

func RunLiveManagerHistorySimulation(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle) (LiveManagerHistoryResult, error) {
	return RunLiveManagerHistorySimulationWithOptions(cfg, btc, assets, LiveManagerHistoryOptions{})
}

func RunLiveManagerHistorySimulationWithOptions(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, opts LiveManagerHistoryOptions) (LiveManagerHistoryResult, error) {
	cfg = simulationConfig(cfg)
	cfg = applyHistoryResearchProfile(cfg, opts)
	btc1d := btc["1d"]
	warmup := 60
	if len(btc1d) < warmup+2 {
		return LiveManagerHistoryResult{}, fmt.Errorf("not enough BTC 1d candles for live manager history simulation; need %d got %d", warmup+2, len(btc1d))
	}
	lastIndex := historyMinLen(btc1d, assets) - 1
	if lastIndex < warmup+1 {
		return LiveManagerHistoryResult{}, fmt.Errorf("not enough aligned asset candles for live manager history simulation; need index %d got %d", warmup+1, lastIndex+1)
	}
	result := LiveManagerHistoryResult{
		GeneratedAt:                       time.Now(),
		PeriodStart:                       btc1d[warmup].OpenTime,
		PeriodEnd:                         btc1d[lastIndex].CloseTime,
		ResearchArmed:                     opts.ResearchArmed,
		ResearchProfile:                   opts.ResearchProfile,
		ResearchExpiryDays:                opts.ResearchExpiryDays,
		ResearchHoldWatch:                 opts.ResearchHoldWatch,
		ResearchHoldPriceAboveDiscountPct: opts.ResearchHoldPriceAboveDiscountPct,
		ProductionArmedProbe:              opts.ProductionArmedProbe,
		PerCoin:                           map[string]LiveManagerHistoryStats{},
		Notes: []string{
			"Historical live manager simulation aligns BTC 1D/4H/1W frames by daily close time; missing 4H/1W frames fall back to 1D for compatibility.",
			"New simulated orders become active from the next candle to avoid same-candle lookahead.",
			"No OKX calls are made; no real order is placed or canceled.",
		},
	}
	if opts.ResearchArmed {
		result.Notes = append(result.Notes, "Research-only: Agent 1 ARMED is treated as ALLOWED inside this historical backtest only; production/live behavior unchanged.")
	}
	if opts.ProductionArmedProbe {
		result.Notes = append(result.Notes, "Production ARMED probe mode: WATCH creates no order, ARMED uses one probe through live manager, ALLOWED uses normal ladder. Historical simulation only; no OKX calls.")
	}
	if strings.EqualFold(opts.ResearchProfile, "flow-soft") {
		result.Notes = append(result.Notes, "Research-only profile flow-soft: asset flow entry filter is disabled inside this historical backtest only; production/live behavior unchanged.")
	}
	if strings.EqualFold(opts.ResearchProfile, "discount-soft") {
		result.Notes = append(result.Notes, "Research-only profile discount-soft: discount zone premium is relaxed to 10% above support inside this historical backtest only; production/live behavior unchanged.")
	}
	if strings.EqualFold(opts.ResearchProfile, "entry-soft") {
		result.Notes = append(result.Notes, "Research-only profile entry-soft: asset flow, discount, rotation, and relative-strength gates are relaxed inside this historical backtest only; production/live behavior unchanged.")
	}
	if opts.ResearchExpiryDays > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("Research-only expiry override: simulated orders expire after %d daily candles inside this historical backtest only; production/live config unchanged.", opts.ResearchExpiryDays))
	}
	if opts.ResearchHoldWatch {
		result.Notes = append(result.Notes, "Research-only hold-through-watch: simulated open orders are kept when plan falls from ACTIVE_LIMIT to WATCH; production/live behavior unchanged.")
	}
	if opts.ResearchHoldPriceAboveDiscountPct > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("Research-only hold above discount: simulated open orders are kept when price is at most %.2f%% above discount zone; production/live behavior unchanged.", opts.ResearchHoldPriceAboveDiscountPct*100))
	}
	for _, symbol := range cfg.Data.Symbols.Assets {
		ensureHistoryStats(&result, symbol)
	}
	openOrders := []historyOpenOrder{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := warmup; i <= lastIndex; i++ {
		eventAt := historyEventTime(btc1d[i].CloseTime)
		processHistoryExpiry(&result, &openOrders, cfg, opts, i, eventAt)
		btcWindow := btcHistoryTimeframeWindow(btc, i)
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		analysis = applyHistoryResearchMode(&result, analysis, opts, eventAt)
		assetWindows := map[string][]market.Candle{}
		for _, symbol := range cfg.Data.Symbols.Assets {
			if len(assets[symbol]) > i {
				assetWindows[symbol] = assets[symbol][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assetWindows, benchmarks)
		recordHistoryPlanBlockers(&result, plan)
		cycleCfg := cfg
		if opts.ResearchHoldWatch && plan.State == agent2.StateWatch {
			cycleCfg.Live.CancelIfPlanNotActive = false
		}
		cycle := ManageLiveOrdersDryRun(context.Background(), cycleCfg, plan, historyOrderStatuses(openOrders), nil, nil, nil, nil, simulationHaltReader{}, true)
		cycle = applyHistoryHoldThroughWatch(cycle, plan, opts, assetWindows)
		result.WindowsTested++
		recordHistoryCycle(&result, cycle, plan)
		applyHistoryManagerResult(&result, &openOrders, cycle, assetWindows, i, eventAt)
		processHistoryFills(&result, &openOrders, assets, i, eventAt)
	}
	finalizeHistoryStats(&result)
	return result, nil
}

func applyHistoryResearchProfile(cfg config.Config, opts LiveManagerHistoryOptions) config.Config {
	switch strings.ToLower(strings.TrimSpace(opts.ResearchProfile)) {
	case "", "none":
		return cfg
	case "flow-soft":
		cfg.Risk.DisableAssetFlowEntryFilter = true
		cfg.Risk.AllowNeutralReclaimEntry = true
		if cfg.Risk.MinAssetFlowBullScore <= 0 || cfg.Risk.MinAssetFlowBullScore > 0.10 {
			cfg.Risk.MinAssetFlowBullScore = 0.10
		}
		return cfg
	case "discount-soft":
		if cfg.Risk.DiscountZonePremiumPct < 0.10 {
			cfg.Risk.DiscountZonePremiumPct = 0.10
		}
		return cfg
	case "entry-soft":
		cfg.Risk.DisableAssetFlowEntryFilter = true
		cfg.Risk.AllowNeutralReclaimEntry = true
		if cfg.Risk.MinAssetFlowBullScore <= 0 || cfg.Risk.MinAssetFlowBullScore > 0.10 {
			cfg.Risk.MinAssetFlowBullScore = 0.10
		}
		if cfg.Risk.DiscountZonePremiumPct < 0.10 {
			cfg.Risk.DiscountZonePremiumPct = 0.10
		}
		cfg.Risk.DisableRotationScoreFilter = true
		cfg.Risk.MaxRotationRank = 0
		cfg.Risk.DisableRelativeStrengthFilter = true
		return cfg
	default:
		return cfg
	}
}

func applyHistoryResearchMode(result *LiveManagerHistoryResult, analysis agent1.MarketAnalysis, opts LiveManagerHistoryOptions, eventAt string) agent1.MarketAnalysis {
	if !opts.ResearchArmed || analysis.ActionPermission != agent1.Armed {
		return analysis
	}
	out := analysis
	out.ActionPermission = agent1.Allowed
	appendHistoryEventRaw(result, LiveManagerHistoryEvent{Time: eventAt, Type: "RESEARCH_ARMED_AS_ALLOWED", Reason: "Agent 1 ARMED treated as ALLOWED for historical research only"})
	return out
}

func applyHistoryHoldThroughWatch(cycle ManagedCycleResult, plan agent2.Plan, opts LiveManagerHistoryOptions, assets map[string][]market.Candle) ManagedCycleResult {
	if !opts.ResearchHoldWatch || plan.State != agent2.StateWatch {
		return cycle
	}
	keptCanceled := []ManagedOrderDecision{}
	keptReplaced := []ManagedOrderDecision{}
	for _, decision := range cycle.Canceled {
		if shouldHistoryHoldWatchDecision(decision) || shouldHistoryHoldAboveDiscount(decision, opts, assets) {
			decision.Action = "keep"
			decision.Reason = historyHoldReason(decision, opts, assets)
			cycle.Kept = append(cycle.Kept, decision)
			continue
		}
		keptCanceled = append(keptCanceled, decision)
	}
	for _, decision := range cycle.Replaced {
		if shouldHistoryHoldWatchDecision(decision) || shouldHistoryHoldAboveDiscount(decision, opts, assets) {
			decision.Action = "keep"
			decision.Reason = historyHoldReason(decision, opts, assets)
			decision.ReplacedOrder = false
			cycle.Kept = append(cycle.Kept, decision)
			continue
		}
		keptReplaced = append(keptReplaced, decision)
	}
	cycle.Canceled = keptCanceled
	cycle.Replaced = keptReplaced
	cycle.Summary = managedSummary(cycle)
	return cycle
}

func shouldHistoryHoldWatchDecision(decision ManagedOrderDecision) bool {
	if decision.Reason == "plan no longer ACTIVE_LIMIT" {
		return true
	}
	if decision.Desired.Symbol != "" {
		return false
	}
	if decision.Order.LayerIndex <= 0 || decision.Order.Price <= 0 {
		return false
	}
	return decision.Reason == "order no longer matches active asset/layer"
}

func shouldHistoryHoldAboveDiscount(decision ManagedOrderDecision, opts LiveManagerHistoryOptions, assets map[string][]market.Candle) bool {
	if opts.ResearchHoldPriceAboveDiscountPct <= 0 || decision.Reason != "order no longer matches active asset/layer" {
		return false
	}
	order := decision.Order
	if order.Price <= 0 {
		return false
	}
	candles := assets[historyDecisionSymbol(decision)]
	if len(candles) < 60 {
		return false
	}
	price := market.LastClose(candles)
	support, _ := market.RangeZone(candles, 60)
	if !support.Valid() || price <= 0 {
		return false
	}
	return price <= support.High*(1+opts.ResearchHoldPriceAboveDiscountPct)
}

func historyHoldReason(decision ManagedOrderDecision, opts LiveManagerHistoryOptions, assets map[string][]market.Candle) string {
	if shouldHistoryHoldAboveDiscount(decision, opts, assets) {
		return "research hold-if-price-above-discount: price still near discount zone"
	}
	return "research hold-through-watch: plan WATCH, keep existing simulated order"
}

func recordHistoryPlanBlockers(result *LiveManagerHistoryResult, plan agent2.Plan) {
	for _, candidate := range plan.Watchlist.Candidates {
		symbol := candidate.Symbol
		if symbol == "" {
			continue
		}
		added := false
		for _, item := range candidate.EntryChecklist {
			if item.Pass {
				continue
			}
			name := item.Name
			if item.Severity != "" {
				name = item.Severity + ":" + name
			}
			incHistoryBlocker(result, symbol, name)
			added = true
		}
		if !added && candidate.BlockReason != "" && candidate.State != agent2.StateActiveLimit {
			incHistoryBlocker(result, symbol, normalizeHistoryBlocker(candidate.BlockReason))
		}
		for _, missing := range candidate.Missing {
			incHistoryBlocker(result, symbol, normalizeHistoryBlocker(missing))
		}
	}
}

func incHistoryBlocker(result *LiveManagerHistoryResult, symbol, blocker string) {
	blocker = strings.TrimSpace(blocker)
	if blocker == "" {
		return
	}
	updateHistoryStats(result, symbol, func(s *LiveManagerHistoryStats) {
		if s.Blockers == nil {
			s.Blockers = map[string]int{}
		}
		s.Blockers[blocker]++
	})
}

func incHistoryCancelReason(stats *LiveManagerHistoryStats, reason string) {
	reason = normalizeHistoryCancelReason(reason)
	if reason == "" {
		return
	}
	if stats.CancelReasons == nil {
		stats.CancelReasons = map[string]int{}
	}
	stats.CancelReasons[reason]++
}

func normalizeHistoryCancelReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(strings.ReplaceAll(reason, " ", "_"))
}

func incHistoryDesiredLoss(stats *LiveManagerHistoryStats, reason string) {
	reason = normalizeHistoryDesiredLoss(reason)
	if reason == "" {
		return
	}
	if stats.DesiredLoss == nil {
		stats.DesiredLoss = map[string]int{}
	}
	stats.DesiredLoss[reason]++
}

func normalizeHistoryDesiredLoss(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(strings.ReplaceAll(reason, " ", "_"))
}

func historyDesiredLossReason(plan agent2.Plan, decision ManagedOrderDecision) string {
	if decision.Desired.Symbol != "" {
		return "LAYER_REPRICED"
	}
	symbol := historyDecisionSymbol(decision)
	for _, asset := range plan.Assets {
		if strings.EqualFold(asset.Symbol, symbol) {
			if asset.State != agent2.StateActiveLimit {
				return asset.Reason
			}
			return "ACTIVE_ASSET_LAYER_MISSING"
		}
	}
	for _, candidate := range plan.Watchlist.Candidates {
		if !strings.EqualFold(candidate.Symbol, symbol) {
			continue
		}
		for _, item := range candidate.EntryChecklist {
			if !item.Pass {
				return item.Severity + ":" + item.Name
			}
		}
		if len(candidate.Missing) > 0 {
			return normalizeHistoryBlocker(candidate.Missing[0])
		}
		if candidate.BlockReason != "" {
			return normalizeHistoryBlocker(candidate.BlockReason)
		}
	}
	if plan.State != agent2.StateActiveLimit {
		return "PLAN_" + string(plan.State)
	}
	return "UNKNOWN"
}

func normalizeHistoryBlocker(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "btc permission"):
		return "BTC_PERMISSION"
	case strings.Contains(lower, "btc risk"):
		return "BTC_RISK_REGIME"
	case strings.Contains(lower, "falling knife") || strings.Contains(lower, "dao rơi"):
		return "FALLING_KNIFE"
	case strings.Contains(lower, "fomo"):
		return "FOMO"
	case strings.Contains(lower, "relative strength"):
		return "RELATIVE_STRENGTH"
	case strings.Contains(lower, "rotation rank"):
		return "ROTATION_RANK"
	case strings.Contains(lower, "rotation score") || strings.Contains(lower, "rotation chưa đạt"):
		return "ROTATION_SCORE"
	case strings.Contains(lower, "asset flow") || strings.Contains(lower, "flow") || strings.Contains(lower, "reclaim") || strings.Contains(lower, "absorption"):
		return "ASSET_FLOW_ENTRY"
	case strings.Contains(lower, "discount") || strings.Contains(lower, "support zone") || strings.Contains(lower, "support quá sâu"):
		return "DISCOUNT_ZONE"
	case strings.Contains(lower, "reward/risk"):
		return "REWARD_RISK"
	case strings.Contains(lower, "chưa đủ dữ liệu"):
		return "DATA_WAIT"
	default:
		return strings.ToUpper(strings.ReplaceAll(reason, " ", "_"))
	}
}

func recordHistoryCycle(result *LiveManagerHistoryResult, cycle ManagedCycleResult, plan agent2.Plan) {
	for _, desired := range cycle.Desired {
		updateHistoryStats(result, desired.Symbol, func(s *LiveManagerHistoryStats) { s.Desired++ })
	}
	for _, decision := range cycle.Kept {
		updateHistoryStats(result, historyDecisionSymbol(decision), func(s *LiveManagerHistoryStats) { s.Kept++ })
	}
	for _, decision := range cycle.Canceled {
		updateHistoryStats(result, historyDecisionSymbol(decision), func(s *LiveManagerHistoryStats) {
			s.Canceled++
			incHistoryCancelReason(s, decision.Reason)
			incHistoryDesiredLoss(s, historyDesiredLossReason(plan, decision))
		})
	}
	for _, decision := range cycle.Replaced {
		updateHistoryStats(result, historyDecisionSymbol(decision), func(s *LiveManagerHistoryStats) {
			s.Replaced++
			incHistoryDesiredLoss(s, historyDesiredLossReason(plan, decision))
		})
	}
	for _, decision := range cycle.Placed {
		updateHistoryStats(result, historyDecisionSymbol(decision), func(s *LiveManagerHistoryStats) { s.Placed++ })
	}
	for _, decision := range cycle.Blocked {
		updateHistoryStats(result, historyDecisionSymbol(decision), func(s *LiveManagerHistoryStats) { s.Blocked++ })
	}
}

func recordArmedProbeCycle(result *LiveManagerHistoryResult, cycle ManagedCycleResult, plan agent2.Plan) {
	if !result.ProductionArmedProbe || plan.State != agent2.StateArmed {
		return
	}
	probeDesired := 0
	for _, desired := range cycle.Desired {
		if strings.EqualFold(desired.AllocationTier, string(OpportunityProbe)) || desired.LayerIndex == 1 {
			probeDesired++
		}
	}
	probePlaced := 0
	for _, decision := range cycle.Placed {
		if strings.EqualFold(decision.Desired.AllocationTier, string(OpportunityProbe)) || decision.LayerIndex == 1 || decision.Desired.LayerIndex == 1 {
			probePlaced++
		}
	}
	result.ArmedProbe.Desired += probeDesired
	result.ArmedProbe.Placed += probePlaced
	result.ArmedProbe.Kept += len(cycle.Kept)
	result.ArmedProbe.Canceled += len(cycle.Canceled)
	result.ArmedProbe.Replaced += len(cycle.Replaced)
	result.ArmedProbe.Blocked += len(cycle.Blocked)
	if result.ArmedProbe.Blockers == nil {
		result.ArmedProbe.Blockers = map[string]int{}
	}
	for _, decision := range cycle.Blocked {
		reason := normalizeHistoryBlocker(decision.Reason)
		result.ArmedProbe.Blockers[reason]++
	}
}

func applyHistoryManagerResult(result *LiveManagerHistoryResult, openOrders *[]historyOpenOrder, cycle ManagedCycleResult, assets map[string][]market.Candle, index int, eventAt string) {
	for _, decision := range cycle.Canceled {
		removeHistoryOrder(openOrders, decision)
		appendHistoryEvent(result, eventAt, decision, "CANCEL", historyCancelDiagnostics(decision, assets))
	}
	for _, decision := range cycle.Replaced {
		removeHistoryOrder(openOrders, decision)
		appendHistoryEvent(result, eventAt, decision, "REPLACE", "")
		if decision.Desired.Symbol != "" {
			*openOrders = append(*openOrders, historyOpenFromDesired(decision.Desired, index))
		}
	}
	for _, decision := range cycle.Placed {
		if decision.Desired.Symbol == "" {
			continue
		}
		*openOrders = append(*openOrders, historyOpenFromDesired(decision.Desired, index))
		appendHistoryEvent(result, eventAt, decision, "PLACE", "")
	}
	for _, decision := range cycle.Blocked {
		appendHistoryEvent(result, eventAt, decision, "BLOCK", "")
	}
}

func processHistoryFills(result *LiveManagerHistoryResult, openOrders *[]historyOpenOrder, assets map[string][]market.Candle, index int, eventAt string) {
	remaining := []historyOpenOrder{}
	for _, order := range *openOrders {
		symbol := orderSymbol(order.Status)
		candles := assets[symbol]
		if len(candles) <= index || index < order.ActiveFromIndex {
			remaining = append(remaining, order)
			continue
		}
		candle := candles[index]
		if candle.Low > 0 && order.Status.Price > 0 && candle.Low <= order.Status.Price {
			updateHistoryStats(result, symbol, func(s *LiveManagerHistoryStats) {
				s.Filled++
				if s.LayerFills == nil {
					s.LayerFills = map[int]int{}
				}
				s.LayerFills[order.Status.LayerIndex]++
			})
			appendHistoryEventFromOrder(result, eventAt, order.Status, "FILL", "candle low crossed limit price")
			continue
		}
		remaining = append(remaining, order)
	}
	*openOrders = remaining
}

func processHistoryExpiry(result *LiveManagerHistoryResult, openOrders *[]historyOpenOrder, cfg config.Config, opts LiveManagerHistoryOptions, index int, eventAt string) {
	expiry := historyExpiryDays(cfg, opts)
	if expiry <= 0 {
		return
	}
	remaining := []historyOpenOrder{}
	for _, order := range *openOrders {
		if index-order.PlacedIndex >= expiry {
			symbol := orderSymbol(order.Status)
			updateHistoryStats(result, symbol, func(s *LiveManagerHistoryStats) { s.Expired++ })
			appendHistoryEventFromOrder(result, eventAt, order.Status, "EXPIRE", "simulated order age exceeded expiry")
			continue
		}
		remaining = append(remaining, order)
	}
	*openOrders = remaining
}

func finalizeHistoryStats(result *LiveManagerHistoryResult) {
	finalizeOneHistoryStats(&result.ArmedProbe)
	for symbol, stats := range result.PerCoin {
		finalizeOneHistoryStats(&stats)
		result.PerCoin[symbol] = stats
	}
	finalizeOneHistoryStats(&result.Total)
	result.Summary = fmt.Sprintf("live manager history: windows=%d placed=%d filled=%d canceled=%d replaced=%d blocked=%d fill_rate=%.1f%% cancel_rate=%.1f%%", result.WindowsTested, result.Total.Placed, result.Total.Filled, result.Total.Canceled, result.Total.Replaced, result.Total.Blocked, result.Total.FillRate*100, result.Total.CancelRate*100)
}

func finalizeOneHistoryStats(stats *LiveManagerHistoryStats) {
	if stats.Placed > 0 {
		stats.FillRate = finiteRate(float64(stats.Filled) / float64(stats.Placed))
		stats.CancelRate = finiteRate(float64(stats.Canceled) / float64(stats.Placed))
		stats.ReplaceRate = finiteRate(float64(stats.Replaced) / float64(stats.Placed))
	}
	stats.QualityScore, stats.QualityGrade, stats.QualityReason = historyQuality(stats)
	bestLayer, bestFills := 0, 0
	for layer, fills := range stats.LayerFills {
		if fills > bestFills || (fills == bestFills && (bestLayer == 0 || layer < bestLayer)) {
			bestLayer = layer
			bestFills = fills
		}
	}
	stats.BestLayer = bestLayer
	if len(stats.Blockers) == 0 {
		stats.Blockers = nil
	}
	if len(stats.CancelReasons) == 0 {
		stats.CancelReasons = nil
	}
	if len(stats.DesiredLoss) == 0 {
		stats.DesiredLoss = nil
	}
}

func finiteRate(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func historyQuality(stats *LiveManagerHistoryStats) (float64, string, string) {
	if stats.Placed == 0 {
		return 0, "NO_SAMPLE", "no historical live-manager orders"
	}
	score := 100.0*stats.FillRate - 40.0*stats.CancelRate - 20.0*stats.ReplaceRate - 10.0*finiteRate(float64(stats.Expired)/float64(stats.Placed))
	if stats.Placed < 5 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	grade := "D"
	switch {
	case score >= 70:
		grade = "A"
	case score >= 50:
		grade = "B"
	case score >= 30:
		grade = "C"
	}
	reason := fmt.Sprintf("fill=%.1f%% cancel=%.1f%% replace=%.1f%% expired=%d placed=%d", stats.FillRate*100, stats.CancelRate*100, stats.ReplaceRate*100, stats.Expired, stats.Placed)
	if stats.Placed < 5 {
		reason += "; small sample penalty"
	}
	return finiteRate(score), grade, reason
}

func historyOpenFromDesired(desired ManagedDesiredOrder, index int) historyOpenOrder {
	qty := desired.Quantity
	if qty <= 0 && desired.Price > 0 {
		qty = desired.Notional / desired.Price
	}
	return historyOpenOrder{Status: live.OrderStatus{InstID: desired.InstID, Symbol: desired.Symbol, ClientOrderID: fmt.Sprintf("hist-%s-%d-%d", strings.ToLower(desired.Symbol), desired.LayerIndex, index), OrderID: fmt.Sprintf("hist-ord-%s-%d-%d", strings.ToLower(desired.Symbol), desired.LayerIndex, index), Status: live.StatusLiveOpen, Side: desired.Side, OrderType: desired.Type, Price: desired.Price, Quantity: qty, Notional: desired.Notional, LayerIndex: desired.LayerIndex, Source: strings.TrimSpace(desired.Source + " " + desired.AllocationTier), InvalidationPrice: desired.InvalidationPrice, DecisionReason: desired.DecisionReason}, PlacedIndex: index, ActiveFromIndex: index + 1}
}

func historyOrderStatuses(openOrders []historyOpenOrder) []live.OrderStatus {
	out := make([]live.OrderStatus, 0, len(openOrders))
	for _, order := range openOrders {
		out = append(out, order.Status)
	}
	return out
}

func removeHistoryOrder(openOrders *[]historyOpenOrder, decision ManagedOrderDecision) {
	key := historyDecisionKey(decision)
	if key == "" {
		return
	}
	out := (*openOrders)[:0]
	removed := false
	for _, order := range *openOrders {
		if !removed && historyOrderKey(order.Status) == key {
			removed = true
			continue
		}
		out = append(out, order)
	}
	*openOrders = out
}

func historyDecisionKey(decision ManagedOrderDecision) string {
	if decision.Symbol != "" && decision.LayerIndex > 0 {
		return managedKey(decision.Symbol, decision.LayerIndex)
	}
	if decision.Desired.Symbol != "" && decision.Desired.LayerIndex > 0 {
		return managedKey(decision.Desired.Symbol, decision.Desired.LayerIndex)
	}
	return historyOrderKey(decision.Order)
}

func historyOrderKey(order live.OrderStatus) string {
	if order.LayerIndex <= 0 {
		return ""
	}
	return managedKey(orderSymbol(order), order.LayerIndex)
}

func historyDecisionSymbol(decision ManagedOrderDecision) string {
	if decision.Symbol != "" {
		return decision.Symbol
	}
	if decision.Desired.Symbol != "" {
		return decision.Desired.Symbol
	}
	return orderSymbol(decision.Order)
}

func ensureHistoryStats(result *LiveManagerHistoryResult, symbol string) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	if result.PerCoin == nil {
		result.PerCoin = map[string]LiveManagerHistoryStats{}
	}
	stats := result.PerCoin[symbol]
	if stats.LayerFills == nil {
		stats.LayerFills = map[int]int{}
	}
	if stats.Blockers == nil {
		stats.Blockers = map[string]int{}
	}
	if stats.CancelReasons == nil {
		stats.CancelReasons = map[string]int{}
	}
	if stats.DesiredLoss == nil {
		stats.DesiredLoss = map[string]int{}
	}
	result.PerCoin[symbol] = stats
	if result.Total.LayerFills == nil {
		result.Total.LayerFills = map[int]int{}
	}
	if result.Total.Blockers == nil {
		result.Total.Blockers = map[string]int{}
	}
	if result.Total.CancelReasons == nil {
		result.Total.CancelReasons = map[string]int{}
	}
	if result.Total.DesiredLoss == nil {
		result.Total.DesiredLoss = map[string]int{}
	}
}

func updateHistoryStats(result *LiveManagerHistoryResult, symbol string, update func(*LiveManagerHistoryStats)) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	ensureHistoryStats(result, symbol)
	coin := result.PerCoin[symbol]
	update(&coin)
	result.PerCoin[symbol] = coin
	update(&result.Total)
}

func appendHistoryEvent(result *LiveManagerHistoryResult, eventAt string, decision ManagedOrderDecision, eventType, diagnostics string) {
	price := decision.Desired.Price
	notional := decision.Desired.Notional
	if price <= 0 {
		price = decision.Order.Price
	}
	if notional <= 0 {
		notional = decision.Order.Notional
	}
	reason := decision.Reason
	if diagnostics != "" {
		reason = strings.TrimSpace(reason + " | " + diagnostics)
	}
	appendHistoryEventRaw(result, LiveManagerHistoryEvent{Time: eventAt, Symbol: historyDecisionSymbol(decision), Type: eventType, Layer: firstNonZeroInt(decision.LayerIndex, decision.Desired.LayerIndex, decision.Order.LayerIndex), Price: price, Notional: notional, Reason: reason})
}

func historyCancelDiagnostics(decision ManagedOrderDecision, assets map[string][]market.Candle) string {
	symbol := historyDecisionSymbol(decision)
	candles := assets[symbol]
	if len(candles) == 0 {
		return ""
	}
	price := market.LastClose(candles)
	support, _ := market.RangeZone(candles, minInt(len(candles), 60))
	if price <= 0 || !support.Valid() {
		return ""
	}
	distance := price/support.High - 1
	return fmt.Sprintf("diag close=%.8f support_high=%.8f distance_above_support=%.2f%% order_price=%.8f", price, support.High, distance*100, decision.Order.Price)
}

func appendHistoryEventFromOrder(result *LiveManagerHistoryResult, eventAt string, order live.OrderStatus, eventType, reason string) {
	appendHistoryEventRaw(result, LiveManagerHistoryEvent{Time: eventAt, Symbol: orderSymbol(order), Type: eventType, Layer: order.LayerIndex, Price: order.Price, Notional: order.Notional, Reason: reason})
}

func appendHistoryEventRaw(result *LiveManagerHistoryResult, event LiveManagerHistoryEvent) {
	if len(result.Events) >= maxHistoryEvents {
		return
	}
	result.Events = append(result.Events, event)
}

func firstNonZeroInt(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func historyExpiryDays(cfg config.Config, opts LiveManagerHistoryOptions) int {
	if opts.ResearchExpiryDays > 0 {
		return opts.ResearchExpiryDays
	}
	if cfg.Live.CancelStaleAfterMinutes > 0 {
		return int(math.Ceil(float64(cfg.Live.CancelStaleAfterMinutes) / (24 * 60)))
	}
	if cfg.Execution.OrderExpiryHours > 0 {
		return int(math.Ceil(float64(cfg.Execution.OrderExpiryHours) / 24.0))
	}
	return 2
}

func historyMinLen(btc []market.Candle, assets map[string][]market.Candle) int {
	min := len(btc)
	for _, candles := range assets {
		if len(candles) < min {
			min = len(candles)
		}
	}
	return min
}

func historyEventTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func SortedHistoryBlockers(blockers map[string]int) []string {
	return sortedHistoryCountKeys(blockers)
}

func SortedHistoryCancelReasons(reasons map[string]int) []string {
	return sortedHistoryCountKeys(reasons)
}

func SortedHistoryDesiredLoss(reasons map[string]int) []string {
	return sortedHistoryCountKeys(reasons)
}

func sortedHistoryCountKeys(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for item := range counts {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] == counts[out[j]] {
			return out[i] < out[j]
		}
		return counts[out[i]] > counts[out[j]]
	})
	return out
}

func SortedHistorySymbols(stats map[string]LiveManagerHistoryStats) []string {
	out := make([]string, 0, len(stats))
	for symbol := range stats {
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}
