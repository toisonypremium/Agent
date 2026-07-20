package backtest

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
)

const maxSimEvents = 200

type Agent2Simulation struct {
	Enabled     bool                     `json:"enabled"`
	Assets      map[string]AssetSimStats `json:"assets"`
	Summary     string                   `json:"summary"`
	Diagnostics Agent2Diagnostics        `json:"diagnostics"`
}

type AssetSimStats struct {
	Symbol        string  `json:"symbol"`
	ScoutPlans    int     `json:"scout_plans"`
	ArmedPlans    int     `json:"armed_plans"`
	PlansCreated  int     `json:"plans_created"`
	OrdersPlaced  int     `json:"orders_placed"`
	OrdersFilled  int     `json:"orders_filled"`
	OrdersExpired int     `json:"orders_expired"`
	Invalidations int     `json:"invalidations"`
	TakeProfits   int     `json:"take_profits"`
	TimeStops     int     `json:"time_stops"`
	FillRate      float64 `json:"fill_rate"`
	MaxDeployed   float64 `json:"max_deployed"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	FinalPnL      float64 `json:"final_pnl"`
}

type Agent2Diagnostics struct {
	WindowsTested         int                       `json:"windows_tested"`
	ScoutCandidates       int                       `json:"scout_candidates"`
	ArmedCandidates       int                       `json:"armed_candidates"`
	ActiveLimitPlans      int                       `json:"active_limit_plans"`
	Agent1PermissionCount map[agent1.Permission]int `json:"agent1_permission_counts"`
	Agent1RegimeCounts    map[string]int            `json:"agent1_regime_counts"`
	Agent1RiskCounts      map[string]int            `json:"agent1_risk_counts"`
	AssetReasonCounts     map[string]map[string]int `json:"asset_reason_counts"`
	HardReasonCounts      map[string]int            `json:"hard_reason_counts"`
	SoftReasonCounts      map[string]int            `json:"soft_reason_counts"`
	Events                []Agent2SimEvent          `json:"events"`
	Notes                 []string                  `json:"notes"`
}

type Agent2SimEvent struct {
	Time         string  `json:"time"`
	Symbol       string  `json:"symbol"`
	Type         string  `json:"type"`
	Layer        int     `json:"layer,omitempty"`
	Price        float64 `json:"price,omitempty"`
	Invalidation float64 `json:"invalidation,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type simOrder struct {
	symbol          string
	layer           int
	price           float64
	quantity        float64
	notional        float64
	invalidation    float64
	placedIndex     int
	activeFromIndex int
}

type simPosition struct {
	qty            float64
	cost           float64
	realized       float64
	invalidation   float64
	firstFillIndex int
}

type SimulationOverrides struct {
	InvalidationBuffer   float64
	LayerDepthMultiplier float64
	TargetSymbols        map[string]bool
	TakeProfitPct        float64
	TimeStopDays         int
	AllowArmedAsAllowed  bool
	SlippageBps          float64
	FillFraction         float64
	FeeBps               float64
}

func RunAgent2Simulation(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle) (Agent2Simulation, error) {
	return RunAgent2SimulationWithOverrides(cfg, btc, assets, SimulationOverrides{})
}

func RunAgent2SimulationWithOverrides(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, overrides SimulationOverrides) (Agent2Simulation, error) {
	btc1d := btc["1d"]
	expiryDays := expiryDays(cfg.Execution.OrderExpiryHours)
	warmup := 60
	need := warmup + expiryDays + 1
	if len(btc1d) < need {
		return Agent2Simulation{}, fmt.Errorf("not enough BTC 1d candles for Agent 2 simulation; need %d got %d", need, len(btc1d))
	}
	sim := Agent2Simulation{Enabled: true, Assets: map[string]AssetSimStats{}, Diagnostics: newAgent2Diagnostics(cfg.Data.Symbols.Assets)}
	openOrders := map[string][]simOrder{}
	positions := map[string]*simPosition{}
	for _, sym := range cfg.Data.Symbols.Assets {
		cs := assets[sym]
		if len(cs) < need {
			sim.Assets[sym] = AssetSimStats{Symbol: sym}
			incAssetReason(&sim, sym, "NOT_ENOUGH_ASSET_1D_CANDLES")
			continue
		}
		sim.Assets[sym] = AssetSimStats{Symbol: sym}
		positions[sym] = &simPosition{}
	}
	if len(sim.Assets) == 0 {
		return Agent2Simulation{}, fmt.Errorf("no assets configured for Agent 2 simulation")
	}

	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < need {
		return sim, nil
	}
	for i := warmup; i <= lastIndex; i++ {
		btcWindow := btcTimeframeWindow(btc, i)
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		sim.Diagnostics.WindowsTested++
		recordAgent1Diagnostics(&sim, analysis)

		planAnalysis := applyResearchPermissionOverride(&sim, analysis, overrides)

		assetWindows := map[string][]market.Candle{}
		for _, sym := range cfg.Data.Symbols.Assets {
			if len(assets[sym]) > i {
				assetWindows[sym] = assets[sym][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, planAnalysis, assetWindows, benchmarks)
		plan = applySimulationOverrides(plan, overrides)
		recordPlanReasons(&sim, cfg, planAnalysis, plan)
		placeOrders(&sim, openOrders, positions, plan, i, eventTime(btc1d[i].CloseTime))
		processOrdersAndPositionsWithOverrides(&sim, openOrders, positions, assets, i, expiryDays, eventTime(btc1d[i].CloseTime), overrides)
	}
	finalizeSimulation(&sim, positions, assets, lastIndex)
	return sim, nil
}

func applyResearchPermissionOverride(sim *Agent2Simulation, analysis agent1.MarketAnalysis, overrides SimulationOverrides) agent1.MarketAnalysis {
	if !overrides.AllowArmedAsAllowed || analysis.ActionPermission != agent1.Armed {
		return analysis
	}
	out := analysis
	out.ActionPermission = agent1.Allowed
	if sim != nil {
		sim.Diagnostics.Notes = append(sim.Diagnostics.Notes, "Research-only: ARMED treated as ALLOWED inside this backtest simulation; production plan/live behavior unchanged.")
	}
	return out
}

func applySimulationOverrides(plan agent2.Plan, overrides SimulationOverrides) agent2.Plan {
	if overrides.InvalidationBuffer <= 0 && overrides.LayerDepthMultiplier <= 0 && len(overrides.TargetSymbols) == 0 {
		return plan
	}
	out := plan
	out.Assets = append([]agent2.AssetPlan(nil), plan.Assets...)
	for i := range out.Assets {
		asset := out.Assets[i]
		if len(overrides.TargetSymbols) > 0 && !overrides.TargetSymbols[asset.Symbol] {
			out.Assets[i] = asset
			continue
		}
		if asset.State != agent2.StateActiveLimit || !asset.DiscountZone.Valid() {
			out.Assets[i] = asset
			continue
		}
		if overrides.InvalidationBuffer > 0 {
			asset.Invalidation = asset.DiscountZone.Low * (1 - overrides.InvalidationBuffer)
		}
		if overrides.LayerDepthMultiplier > 0 {
			for j := range asset.Layers {
				base := asset.Layers[j].Price
				distance := asset.DiscountZone.High - base
				newPrice := asset.DiscountZone.High - distance*overrides.LayerDepthMultiplier
				if newPrice <= 0 {
					newPrice = base
				}
				asset.Layers[j].Price = newPrice
				if newPrice > 0 {
					asset.Layers[j].Quantity = asset.Layers[j].Notional / newPrice
				}
			}
		}
		out.Assets[i] = asset
	}
	return out
}

func newAgent2Diagnostics(symbols []string) Agent2Diagnostics {
	d := Agent2Diagnostics{
		Agent1PermissionCount: map[agent1.Permission]int{},
		Agent1RegimeCounts:    map[string]int{},
		Agent1RiskCounts:      map[string]int{},
		AssetReasonCounts:     map[string]map[string]int{},
		HardReasonCounts:      map[string]int{},
		SoftReasonCounts:      map[string]int{},
		Notes: []string{
			"Historical Agent 1 simulation uses BTC 1D candles as fallback for 4H and 1W; this keeps backtest local but is not true multi-timeframe alignment.",
			"Limit orders become active from the next candle to avoid same-candle lookahead.",
			"Conservative OHLCV ambiguity rule: if take-profit and invalidation are both crossed in one candle, invalidation wins.",
		},
	}
	for _, sym := range symbols {
		d.AssetReasonCounts[sym] = map[string]int{}
	}
	return d
}

func recordAgent1Diagnostics(sim *Agent2Simulation, analysis agent1.MarketAnalysis) {
	sim.Diagnostics.Agent1PermissionCount[analysis.ActionPermission]++
	sim.Diagnostics.Agent1RegimeCounts[analysis.MarketRegime]++
	riskKey := fmt.Sprintf("risk=%s falling=%s fomo=%s", analysis.RiskLevel, analysis.FallingKnifeRisk, analysis.FomoRisk)
	sim.Diagnostics.Agent1RiskCounts[riskKey]++
}

func recordPlanReasons(sim *Agent2Simulation, cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan) {
	if len(plan.Assets) == 0 && analysis.ActionPermission != agent1.Allowed {
		for _, sym := range cfg.Data.Symbols.Assets {
			if sim.Assets[sym].Symbol != "" {
				incAssetReason(sim, sym, "BTC_PERMISSION_"+string(analysis.ActionPermission))
			}
		}
		return
	}
	seen := map[string]bool{}
	for _, asset := range plan.Assets {
		if sim.Assets[asset.Symbol].Symbol == "" {
			continue
		}
		seen[asset.Symbol] = true
		stats := sim.Assets[asset.Symbol]
		switch asset.State {
		case agent2.StateScout:
			stats.ScoutPlans++
			sim.Diagnostics.ScoutCandidates++
		case agent2.StateArmed:
			stats.ArmedPlans++
			sim.Diagnostics.ArmedCandidates++
		case agent2.StateActiveLimit:
			sim.Diagnostics.ActiveLimitPlans++
		}
		sim.Assets[asset.Symbol] = stats
		if len(asset.Reasons) > 0 {
			for _, reason := range asset.Reasons {
				key := string(reason.Code)
				if key == "" {
					key = reason.Message
				}
				if key == "" {
					continue
				}
				incAssetReason(sim, asset.Symbol, key)
				switch reason.Severity {
				case agent2.ReasonHardBlock:
					sim.Diagnostics.HardReasonCounts[key]++
				case agent2.ReasonSoftWait:
					sim.Diagnostics.SoftReasonCounts[key]++
				}
			}
			continue
		}
		reason := asset.Reason
		if reason == "" {
			reason = string(asset.State)
		}
		incAssetReason(sim, asset.Symbol, reason)
	}
	for _, sym := range cfg.Data.Symbols.Assets {
		if sim.Assets[sym].Symbol != "" && !seen[sym] {
			incAssetReason(sim, sym, "NO_ASSET_PLAN_RETURNED")
		}
	}
}

func incAssetReason(sim *Agent2Simulation, sym, reason string) {
	if sim.Diagnostics.AssetReasonCounts == nil {
		sim.Diagnostics.AssetReasonCounts = map[string]map[string]int{}
	}
	if sim.Diagnostics.AssetReasonCounts[sym] == nil {
		sim.Diagnostics.AssetReasonCounts[sym] = map[string]int{}
	}
	sim.Diagnostics.AssetReasonCounts[sym][reason]++
}

func appendEvent(sim *Agent2Simulation, event Agent2SimEvent) {
	if len(sim.Diagnostics.Events) >= maxSimEvents {
		return
	}
	sim.Diagnostics.Events = append(sim.Diagnostics.Events, event)
}

func placeOrders(sim *Agent2Simulation, openOrders map[string][]simOrder, positions map[string]*simPosition, plan agent2.Plan, index int, eventAt string) {
	for _, asset := range plan.Assets {
		stats := sim.Assets[asset.Symbol]
		if stats.Symbol == "" {
			continue
		}
		if asset.State != agent2.StateActiveLimit {
			sim.Assets[asset.Symbol] = stats
			continue
		}
		stats.PlansCreated++
		appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: asset.Symbol, Type: "PLAN", Invalidation: asset.Invalidation, Reason: asset.Reason})
		pos := positions[asset.Symbol]
		if pos != nil && pos.qty > 0 {
			sim.Assets[asset.Symbol] = stats
			continue
		}
		if len(openOrders[asset.Symbol]) > 0 {
			sim.Assets[asset.Symbol] = stats
			continue
		}
		for _, layer := range asset.Layers {
			openOrders[asset.Symbol] = append(openOrders[asset.Symbol], simOrder{symbol: asset.Symbol, layer: layer.Index, price: layer.Price, quantity: layer.Quantity, notional: layer.Notional, invalidation: asset.Invalidation, placedIndex: index, activeFromIndex: index + 1})
			stats.OrdersPlaced++
		}
		sim.Assets[asset.Symbol] = stats
	}
}

func processOrdersAndPositions(sim *Agent2Simulation, openOrders map[string][]simOrder, positions map[string]*simPosition, assets map[string][]market.Candle, index, expiryDays int, eventAt string) {
	processOrdersAndPositionsWithOverrides(sim, openOrders, positions, assets, index, expiryDays, eventAt, SimulationOverrides{})
}

func processOrdersAndPositionsWithOverrides(sim *Agent2Simulation, openOrders map[string][]simOrder, positions map[string]*simPosition, assets map[string][]market.Candle, index, expiryDays int, eventAt string, overrides SimulationOverrides) {
	for sym, orders := range openOrders {
		if len(assets[sym]) <= index {
			continue
		}
		candle := assets[sym][index]
		stats := sim.Assets[sym]
		remaining := []simOrder{}
		closed := false
		for _, order := range orders {
			if index < order.activeFromIndex {
				remaining = append(remaining, order)
				continue
			}
			if index-order.placedIndex >= expiryDays {
				stats.OrdersExpired++
				appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: sym, Type: "EXPIRE", Layer: order.layer, Price: order.price})
				continue
			}
			if candle.Low <= order.price {
				fillFraction := overrides.FillFraction
				if fillFraction <= 0 || fillFraction > 1 {
					fillFraction = 1
				}
				fillPrice := order.price * (1 + math.Max(0, overrides.SlippageBps)/10000)
				fillQty := order.quantity * fillFraction
				fillCost := fillPrice * fillQty * (1 + math.Max(0, overrides.FeeBps)/10000)
				pos := positions[sym]
				if pos == nil {
					pos = &simPosition{}
					positions[sym] = pos
				}
				if pos.qty == 0 {
					pos.firstFillIndex = index
				}
				pos.qty += fillQty
				pos.cost += fillCost
				pos.invalidation = order.invalidation
				stats.OrdersFilled++
				appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: sym, Type: "FILL", Layer: order.layer, Price: fillPrice, Invalidation: order.invalidation})
				if fillFraction < 1 {
					order.quantity -= fillQty
					order.notional = order.price * order.quantity
					remaining = append(remaining, order)
				}
				if pos.cost > stats.MaxDeployed {
					stats.MaxDeployed = pos.cost
				}
				continue
			}
			remaining = append(remaining, order)
		}
		pos := positions[sym]
		if pos != nil && pos.qty > 0 {
			avg := pos.cost / pos.qty
			dd := (candle.Low - avg) / avg
			if dd < stats.MaxDrawdown {
				stats.MaxDrawdown = dd
			}
			tp := 0.0
			if overrides.TakeProfitPct > 0 && index > pos.firstFillIndex {
				tp = avg * (1 + overrides.TakeProfitPct)
			}
			exit := ConservativeOHLCVExitDecision(candle, avg, pos.invalidation, tp)
			switch exit.Exit {
			case OHLCVExitInvalidation:
				stats.Invalidations++
				pos.realized += (pos.invalidation - avg) * pos.qty
				appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: sym, Type: "INVALIDATION", Price: pos.invalidation, Invalidation: pos.invalidation, Reason: exit.Reason})
				pos.qty = 0
				pos.cost = 0
				closed = true
			case OHLCVExitTakeProfit:
				stats.TakeProfits++
				pos.realized += (tp - avg) * pos.qty
				appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: sym, Type: "TAKE_PROFIT", Price: tp, Invalidation: pos.invalidation, Reason: fmt.Sprintf("take_profit_pct=%.4f", overrides.TakeProfitPct)})
				pos.qty = 0
				pos.cost = 0
				closed = true
			}
			if !closed && overrides.TimeStopDays > 0 && index-pos.firstFillIndex >= overrides.TimeStopDays {
				stats.TimeStops++
				pos.realized += (candle.Close - avg) * pos.qty
				appendEvent(sim, Agent2SimEvent{Time: eventAt, Symbol: sym, Type: "TIME_STOP", Price: candle.Close, Invalidation: pos.invalidation, Reason: fmt.Sprintf("time_stop_days=%d", overrides.TimeStopDays)})
				pos.qty = 0
				pos.cost = 0
				closed = true
			}
		}
		if closed {
			remaining = nil
		}
		openOrders[sym] = remaining
		sim.Assets[sym] = stats
	}
}

func finalizeSimulation(sim *Agent2Simulation, positions map[string]*simPosition, assets map[string][]market.Candle, lastIndex int) {
	totalPlaced, totalFilled, totalInvalidations := 0, 0, 0
	for sym, stats := range sim.Assets {
		pos := positions[sym]
		if pos != nil {
			stats.FinalPnL = pos.realized
			if pos.qty > 0 && len(assets[sym]) > lastIndex {
				lastClose := assets[sym][lastIndex].Close
				stats.FinalPnL += (lastClose * pos.qty) - pos.cost
			}
		}
		if stats.OrdersPlaced > 0 {
			stats.FillRate = float64(stats.OrdersFilled) / float64(stats.OrdersPlaced)
		}
		if math.IsNaN(stats.FinalPnL) || math.IsInf(stats.FinalPnL, 0) {
			stats.FinalPnL = 0
		}
		totalPlaced += stats.OrdersPlaced
		totalFilled += stats.OrdersFilled
		totalInvalidations += stats.Invalidations
		sim.Assets[sym] = stats
	}
	sim.Summary = fmt.Sprintf("Agent 2 simulation: placed=%d filled=%d invalidations=%d", totalPlaced, totalFilled, totalInvalidations)
}

func expiryDays(hours int) int {
	if hours <= 0 {
		return 1
	}
	return int(math.Ceil(float64(hours) / 24.0))
}

func minLen(btc []market.Candle, assets map[string][]market.Candle) int {
	min := len(btc)
	for _, cs := range assets {
		if len(cs) < min {
			min = len(cs)
		}
	}
	return min
}

func eventTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func topReasons(reasons map[string]int, limit int) string {
	if len(reasons) == 0 {
		return "none"
	}
	type pair struct {
		key string
		val int
	}
	pairs := make([]pair, 0, len(reasons))
	for key, val := range reasons {
		pairs = append(pairs, pair{key: key, val: val})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].val == pairs[j].val {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].val > pairs[j].val
	})
	if limit > 0 && len(pairs) > limit {
		pairs = pairs[:limit]
	}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s=%d", p.key, p.val)
	}
	return strings.Join(parts, "; ")
}
