package liveguard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

type LiveManagerSimulationResult struct {
	GeneratedAt time.Time                   `json:"generated_at"`
	Passed      bool                        `json:"passed"`
	Scenarios   []LiveManagerScenarioResult `json:"scenarios"`
	Summary     string                      `json:"summary"`
}

type LiveManagerScenarioResult struct {
	Name     string             `json:"name"`
	Expected string             `json:"expected"`
	Passed   bool               `json:"passed"`
	Failure  string             `json:"failure,omitempty"`
	Result   ManagedCycleResult `json:"result"`
}

type simulationHaltReader struct{ halted bool }

func (s simulationHaltReader) IsHalted() (bool, error) { return s.halted, nil }

func RunLiveManagerSimulation(cfg config.Config) LiveManagerSimulationResult {
	cfg = simulationConfig(cfg)
	scenarios := []LiveManagerScenarioResult{
		simScenario("WATCH does nothing", "no desired orders and no actions", cfg, agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch}, nil, func(r ManagedCycleResult) bool {
			return len(r.Desired) == 0 && len(r.Placed) == 0 && len(r.Canceled) == 0
		}),
		simScenario("ACTIVE_LIMIT single asset places layers", "would place ETH layers or block ETH only if D-grade", cfg, simPlan("ETHUSDT"), nil, func(r ManagedCycleResult) bool {
			return (len(r.Placed) >= 1 || blockedSymbolReason(r, "ETHUSDT", "canary quality filter blocked D-grade coin")) && len(r.Canceled) == 0
		}),
		simScenario("ACTIVE_LIMIT multi asset places multiple coins", "would place/probe ETH/SOL or block D-grade coins", cfg, simPlan("ETHUSDT", "SOLUSDT"), nil, func(r ManagedCycleResult) bool {
			return placedSymbol(r, "ETHUSDT") && (placedSymbol(r, "SOLUSDT") || blockedSymbolReason(r, "SOLUSDT", "canary quality filter blocked D-grade coin"))
		}),
		simScenario("Matching open order is kept", "keeps matching ETH layer unless ETH is D-grade blocked", cfg, simPlan("ETHUSDT"), []live.OrderStatus{simOpen("ETHUSDT", 1, 100)}, func(r ManagedCycleResult) bool {
			return (len(r.Kept) == 1 && len(r.Canceled) == 0) || (len(r.Canceled) == 1 && blockedSymbolReason(r, "ETHUSDT", "canary quality filter blocked D-grade coin"))
		}),
		simScenario("Inactive plan cancels open order", "would cancel stale ETH order when plan WATCH", cfg, agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch}, []live.OrderStatus{simOpen("ETHUSDT", 1, 100)}, func(r ManagedCycleResult) bool {
			return len(r.Canceled) == 1
		}),
		simScenario("Price drift replaces order", "would replace ETH layer unless ETH is D-grade blocked", cfg, simPlan("ETHUSDT"), []live.OrderStatus{simOpen("ETHUSDT", 1, 80)}, func(r ManagedCycleResult) bool {
			return len(r.Replaced) == 1 || (len(r.Canceled) == 1 && blockedSymbolReason(r, "ETHUSDT", "canary quality filter blocked D-grade coin"))
		}),
		simScenario("Total open cap blocks excess", "blocks excess layers after dynamic opportunity allocation", capOneConfig(cfg), simPlan("ETHUSDT", "SOLUSDT"), nil, func(r ManagedCycleResult) bool {
			return len(r.Placed) == 1 && len(r.Blocked) > 0
		}),
	}
	out := LiveManagerSimulationResult{GeneratedAt: time.Now(), Passed: true, Scenarios: scenarios}
	for _, s := range scenarios {
		if !s.Passed {
			out.Passed = false
			break
		}
	}
	status := "PASS"
	if !out.Passed {
		status = "FAIL"
	}
	out.Summary = fmt.Sprintf("%s: %d live manager scenarios", status, len(out.Scenarios))
	return out
}

func simScenario(name, expected string, cfg config.Config, plan agent2.Plan, open []live.OrderStatus, check func(ManagedCycleResult) bool) LiveManagerScenarioResult {
	result := ManageLiveOrdersDryRun(context.Background(), cfg, plan, open, nil, nil, nil, nil, simulationHaltReader{}, true)
	passed := check(result)
	failure := ""
	if !passed {
		failure = fmt.Sprintf("got desired=%d kept=%d canceled=%d replaced=%d placed=%d blocked=%d", len(result.Desired), len(result.Kept), len(result.Canceled), len(result.Replaced), len(result.Placed), len(result.Blocked))
	}
	return LiveManagerScenarioResult{Name: name, Expected: expected, Passed: passed, Failure: failure, Result: result}
}

func simulationConfig(cfg config.Config) config.Config {
	cfg.Live.Enabled = true
	cfg.Live.AutoExecute = true
	cfg.Live.OrderManagementEnabled = true
	cfg.Live.CanaryMode = true
	if cfg.Live.CanaryMaxNotionalUSDT <= 0 {
		cfg.Live.CanaryMaxNotionalUSDT = 2
	}
	if cfg.Live.MaxOrderNotionalUSDT <= 0 {
		cfg.Live.MaxOrderNotionalUSDT = 10
	}
	cfg.Live.RequirePostOnly = true
	cfg.Live.CancelIfPlanNotActive = true
	if cfg.Live.ReplaceIfPriceDriftPct <= 0 {
		cfg.Live.ReplaceIfPriceDriftPct = 0.01
	}
	if cfg.Live.MaxAutoLayersPerAsset <= 0 {
		cfg.Live.MaxAutoLayersPerAsset = 3
	}
	if cfg.Live.MaxOpenLiveOrdersPerAsset <= 0 {
		cfg.Live.MaxOpenLiveOrdersPerAsset = 3
	}
	if cfg.Live.MaxOpenLiveOrdersTotal <= 0 {
		cfg.Live.MaxOpenLiveOrdersTotal = 9
	}
	if cfg.Live.MaxLiveNotionalPerOrderUSDT <= 0 {
		cfg.Live.MaxLiveNotionalPerOrderUSDT = 2
	}
	if cfg.Live.MaxLiveNotionalPerAssetUSDT <= 0 {
		cfg.Live.MaxLiveNotionalPerAssetUSDT = 6
	}
	if cfg.Live.MaxLiveNotionalTotalUSDT <= 0 {
		cfg.Live.MaxLiveNotionalTotalUSDT = 18
	}
	if cfg.Portfolio.Allocation == nil {
		cfg.Portfolio.Allocation = map[string]float64{}
	}
	for _, symbol := range []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"} {
		if cfg.Portfolio.Allocation[symbol] <= 0 {
			cfg.Portfolio.Allocation[symbol] = 0.33
		}
	}
	if len(cfg.Data.Symbols.Assets) == 0 {
		cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	}
	return cfg
}

func capOneConfig(cfg config.Config) config.Config {
	cfg.Live.MaxOpenLiveOrdersTotal = 1
	return cfg
}

func simPlan(symbols ...string) agent2.Plan {
	assets := []agent2.AssetPlan{}
	for _, symbol := range symbols {
		symbol = strings.ToUpper(symbol)
		price := 100.0
		if symbol == "SOLUSDT" {
			price = 50
		}
		assets = append(assets, agent2.AssetPlan{Symbol: symbol, State: agent2.StateActiveLimit, DiscountZone: market.Zone{Low: price * 0.9, High: price}, Invalidation: price * 0.88, RewardRisk: 3.5, Reason: "simulation active layer", Layers: []agent2.Layer{{Index: 1, Price: price, Notional: 10}, {Index: 2, Price: price * 0.95, Notional: 10}, {Index: 3, Price: price * 0.9, Notional: 10}}})
	}
	return agent2.Plan{Timestamp: time.Now(), State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed, Assets: assets, Summary: "simulation ACTIVE_LIMIT"}
}

func simOpen(symbol string, layer int, price float64) live.OrderStatus {
	qty := 2 / price
	return live.OrderStatus{InstID: live.OKXInstID(symbol), Symbol: strings.ToUpper(symbol), ClientOrderID: fmt.Sprintf("sim-%s-%d", strings.ToLower(symbol), layer), OrderID: fmt.Sprintf("ord-%s-%d", strings.ToLower(symbol), layer), Status: live.StatusLiveOpen, Side: "BUY", OrderType: "limit", Price: price, Quantity: qty, Notional: price * qty, LayerIndex: layer, SubmittedAt: time.Now().Add(-30 * time.Minute).Unix()}
}

func placedSymbol(r ManagedCycleResult, symbol string) bool {
	symbol = strings.ToUpper(symbol)
	for _, p := range r.Placed {
		if strings.ToUpper(p.Symbol) == symbol || strings.ToUpper(p.Desired.Symbol) == symbol {
			return true
		}
	}
	return false
}

func placedOrBlockedByCanaryQuality(r ManagedCycleResult, symbol string) bool {
	return placedSymbol(r, symbol) || blockedByCanaryQuality(r, symbol)
}

func blockedByCanaryQuality(r ManagedCycleResult, symbol string) bool {
	return blockedSymbolReason(r, symbol, "canary quality filter blocked D-grade coin") || blockedSymbolReason(r, symbol, "canary quality filter blocked NO_SAMPLE coin")
}

func blockedSymbolReason(r ManagedCycleResult, symbol, reason string) bool {
	symbol = strings.ToUpper(symbol)
	for _, b := range r.Blocked {
		if strings.ToUpper(b.Symbol) == symbol && b.Reason == reason {
			return true
		}
	}
	return false
}
