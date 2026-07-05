package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func simConfig() config.Config {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.5, "SOLUSDT": 0.3, "RENDERUSDT": 0.2}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.7
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	cfg.Risk.MinRewardRisk = 3
	cfg.Execution.LayerDistribution = []float64{0.25, 0.35, 0.40}
	cfg.Execution.OrderExpiryHours = 48
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.BTCCycle.StressPriceReference = 50
	return cfg
}

func simCandles(symbol string, n int, price float64) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		p := price + float64(i%20)
		out[i] = market.Candle{Symbol: symbol, Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: p, High: p + 2, Low: p - 2, Close: p + 1, Volume: 1000}
	}
	return out
}

func activePlan() agent2.Plan {
	return agent2.Plan{Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Invalidation: 90, Reason: "test active", Layers: []agent2.Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}, {Index: 2, Price: 95, Quantity: 1, Notional: 95}}}}}
}

func newTestSim() Agent2Simulation {
	return Agent2Simulation{Enabled: true, Assets: map[string]AssetSimStats{"ETHUSDT": {Symbol: "ETHUSDT"}}, Diagnostics: newAgent2Diagnostics([]string{"ETHUSDT"})}
}

func TestRunAgent2SimulationRequiresData(t *testing.T) {
	_, err := RunAgent2Simulation(simConfig(), map[string][]market.Candle{"1d": simCandles("BTCUSDT", 10, 100)}, map[string][]market.Candle{"ETHUSDT": simCandles("ETHUSDT", 10, 100)})
	if err == nil {
		t.Fatal("expected not enough data error")
	}
}

func TestSimLimitFillAndPnL(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, Close: 125}, {Low: 99, Close: 110}}}
	processOrdersAndPositions(&sim, orders, positions, assets, 0, 2, "2026-01-01")
	processOrdersAndPositions(&sim, orders, positions, assets, 1, 2, "2026-01-02")
	finalizeSimulation(&sim, positions, assets, 1)
	stats := sim.Assets["ETHUSDT"]
	if stats.OrdersPlaced == 0 || stats.OrdersFilled == 0 || stats.MaxDeployed == 0 || stats.FinalPnL <= 0 {
		t.Fatalf("expected filled profitable simulation: %+v", stats)
	}
}

func TestSimPreventsSameCandleLookaheadFill(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 99, Close: 110}, {Low: 120, Close: 125}}}
	processOrdersAndPositions(&sim, orders, positions, assets, 0, 2, "2026-01-01")
	processOrdersAndPositions(&sim, orders, positions, assets, 1, 2, "2026-01-02")
	stats := sim.Assets["ETHUSDT"]
	if stats.OrdersFilled != 0 {
		t.Fatalf("same-candle low must not fill next-candle order: %+v", stats)
	}
}

func TestSimInvalidation(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, Close: 125}, {Low: 99, Close: 100}, {Low: 89, Close: 91}}}
	processOrdersAndPositions(&sim, orders, positions, assets, 0, 2, "2026-01-01")
	processOrdersAndPositions(&sim, orders, positions, assets, 1, 2, "2026-01-02")
	processOrdersAndPositions(&sim, orders, positions, assets, 2, 2, "2026-01-03")
	finalizeSimulation(&sim, positions, assets, 2)
	stats := sim.Assets["ETHUSDT"]
	if stats.Invalidations == 0 || stats.FinalPnL >= 0 {
		t.Fatalf("expected invalidation loss: %+v", stats)
	}
}

func TestSimExpiryBeforeFill(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, Close: 125}, {Low: 99, Close: 126}}}
	processOrdersAndPositions(&sim, orders, positions, assets, 0, 1, "2026-01-01")
	processOrdersAndPositions(&sim, orders, positions, assets, 1, 1, "2026-01-02")
	stats := sim.Assets["ETHUSDT"]
	if stats.OrdersExpired == 0 || stats.OrdersFilled != 0 {
		t.Fatalf("expected expiry before fill at expiry boundary: %+v", stats)
	}
}

func TestSimInvalidationCancelsOutstandingOrders(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	plan := agent2.Plan{Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Invalidation: 90, Reason: "test active", Layers: []agent2.Layer{{Index: 1, Price: 100, Quantity: 1, Notional: 100}, {Index: 2, Price: 80, Quantity: 1, Notional: 80}}}}}
	placeOrders(&sim, orders, positions, plan, 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, Close: 125}, {Low: 99, Close: 100}, {Low: 89, Close: 91}, {Low: 79, Close: 85}}}
	processOrdersAndPositions(&sim, orders, positions, assets, 1, 5, "2026-01-02")
	processOrdersAndPositions(&sim, orders, positions, assets, 2, 5, "2026-01-03")
	processOrdersAndPositions(&sim, orders, positions, assets, 3, 5, "2026-01-04")
	stats := sim.Assets["ETHUSDT"]
	if stats.Invalidations != 1 || stats.OrdersFilled != 1 || len(orders["ETHUSDT"]) != 0 {
		t.Fatalf("expected invalidation to cancel remaining orders: stats=%+v orders=%+v", stats, orders["ETHUSDT"])
	}
}

func TestSimTakeProfitAfterFill(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, High: 125, Close: 125}, {Low: 99, High: 101, Close: 100}, {Low: 100, High: 106, Close: 105}}}
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 0, 5, "2026-01-01", SimulationOverrides{TakeProfitPct: 0.05})
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 1, 5, "2026-01-02", SimulationOverrides{TakeProfitPct: 0.05})
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 2, 5, "2026-01-03", SimulationOverrides{TakeProfitPct: 0.05})
	finalizeSimulation(&sim, positions, assets, 2)
	stats := sim.Assets["ETHUSDT"]
	if stats.TakeProfits != 1 || stats.FinalPnL <= 0 {
		t.Fatalf("expected take profit with positive pnl: %+v", stats)
	}
}

func TestSimTakeProfitNotSameCandleAsFill(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, High: 125, Close: 125}, {Low: 99, High: 110, Close: 100}}}
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 1, 5, "2026-01-02", SimulationOverrides{TakeProfitPct: 0.05})
	stats := sim.Assets["ETHUSDT"]
	if stats.TakeProfits != 0 || stats.OrdersFilled == 0 {
		t.Fatalf("expected fill but no same-candle TP: %+v", stats)
	}
}

func TestSimInvalidationWinsWhenSameCandleAsTakeProfit(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, High: 125, Close: 125}, {Low: 99, High: 101, Close: 100}, {Low: 89, High: 110, Close: 100}}}
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 1, 5, "2026-01-02", SimulationOverrides{TakeProfitPct: 0.05})
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 2, 5, "2026-01-03", SimulationOverrides{TakeProfitPct: 0.05})
	finalizeSimulation(&sim, positions, assets, 2)
	stats := sim.Assets["ETHUSDT"]
	if stats.Invalidations != 1 || stats.TakeProfits != 0 || stats.FinalPnL >= 0 {
		t.Fatalf("expected invalidation before TP on ambiguous candle: %+v", stats)
	}
}

func TestSimTimeStop(t *testing.T) {
	sim := newTestSim()
	orders := map[string][]simOrder{}
	positions := map[string]*simPosition{"ETHUSDT": {}}
	placeOrders(&sim, orders, positions, activePlan(), 0, "2026-01-01")
	assets := map[string][]market.Candle{"ETHUSDT": {{Low: 120, High: 125, Close: 125}, {Low: 99, High: 101, Close: 100}, {Low: 96, High: 102, Close: 103}}}
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 1, 5, "2026-01-02", SimulationOverrides{TimeStopDays: 1})
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 2, 5, "2026-01-03", SimulationOverrides{TimeStopDays: 1})
	finalizeSimulation(&sim, positions, assets, 2)
	stats := sim.Assets["ETHUSDT"]
	if stats.TimeStops != 1 || stats.FinalPnL <= 0 {
		t.Fatalf("expected profitable time stop: %+v", stats)
	}
}

func TestDiagnosticsRecordBlockedReasons(t *testing.T) {
	sim := newTestSim()
	cfg := simConfig()
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Watch}
	recordAgent1Diagnostics(&sim, analysis)
	recordPlanReasons(&sim, cfg, analysis, agent2.Plan{})
	if sim.Diagnostics.Agent1PermissionCount[agent1.Watch] != 1 {
		t.Fatalf("expected watch permission count: %+v", sim.Diagnostics.Agent1PermissionCount)
	}
	if sim.Diagnostics.AssetReasonCounts["ETHUSDT"]["BTC_PERMISSION_WATCH"] != 1 {
		t.Fatalf("expected BTC permission block reason: %+v", sim.Diagnostics.AssetReasonCounts)
	}
}

func TestResearchPermissionOverridePromotesOnlyArmed(t *testing.T) {
	sim := newTestSim()
	armed := agent1.MarketAnalysis{ActionPermission: agent1.Armed}
	got := applyResearchPermissionOverride(&sim, armed, SimulationOverrides{AllowArmedAsAllowed: true})
	if got.ActionPermission != agent1.Allowed {
		t.Fatalf("ARMED should be research-promoted to ALLOWED: %+v", got.ActionPermission)
	}
	watch := agent1.MarketAnalysis{ActionPermission: agent1.Watch}
	got = applyResearchPermissionOverride(&sim, watch, SimulationOverrides{AllowArmedAsAllowed: true})
	if got.ActionPermission != agent1.Watch {
		t.Fatalf("WATCH must not be promoted: %+v", got.ActionPermission)
	}
	noTrade := agent1.MarketAnalysis{ActionPermission: agent1.NoTrade}
	got = applyResearchPermissionOverride(&sim, noTrade, SimulationOverrides{AllowArmedAsAllowed: true})
	if got.ActionPermission != agent1.NoTrade {
		t.Fatalf("NO_TRADE must not be promoted: %+v", got.ActionPermission)
	}
}

func TestResearchPermissionOverrideAddsSafetyNote(t *testing.T) {
	sim := newTestSim()
	_ = applyResearchPermissionOverride(&sim, agent1.MarketAnalysis{ActionPermission: agent1.Armed}, SimulationOverrides{AllowArmedAsAllowed: true})
	if len(sim.Diagnostics.Notes) == 0 {
		t.Fatalf("expected research-only safety note")
	}
}
