package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
)

func TestManagedBuyAcceptedTimeoutDoesNotSubmitAgain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "unknown-outcome.sqlite")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	writeUnknownOutcomeQualityReport(t)

	exchange := &acceptedThenTimeoutExchange{}
	cfg := unknownOutcomeConfig()
	plan := unknownOutcomePlan()
	execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: "ACCUMULATION_CONFIRMED", FirstOrderDryRunApproved: true}

	first := liveguard.ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, nil, nil, nil, exchange, exchange, alwaysRunningHaltReader{}, execCtx, db, false)
	if len(exchange.accepted) != 1 {
		t.Fatalf("first cycle must reach exchange exactly once: calls=%d result=%+v", len(exchange.accepted), first)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	second := liveguard.ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, open, nil, nil, exchange, exchange, alwaysRunningHaltReader{}, execCtx, db, false)
	t.Logf("cycles: first=%s second=%s calls=%d open=%+v", first.Status, second.Status, len(exchange.accepted), open)
	if len(exchange.accepted) != 1 {
		t.Fatalf("ambiguous accepted BUY was submitted again: calls=%d open_after_restart=%+v first=%+v second=%+v", len(exchange.accepted), open, first, second)
	}
}

func TestHermesBuyAcceptedTimeoutDoesNotSubmitAgain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hermes-unknown-outcome.sqlite")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	exchange := &acceptedThenTimeoutExchange{}
	cfg := unknownOutcomeConfig()
	cfg.HermesOperator.Enabled = true
	cfg.HermesOperator.Mode = "canary"
	cfg.Live.FirstOrderRequireDryRun = false
	plan := agent2.Plan{State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed}
	desired := liveguard.ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, PostOnly: true, Source: "HERMES_OPERATOR", DecisionID: "accepted-timeout", Intent: "PROBE_LIMIT", AllocationTier: string(liveguard.OpportunityProbe)}
	execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: "ACCUMULATION_CONFIRMED", FirstOrderDryRunApproved: true, HermesMode: "canary"}

	first := liveguard.ExecuteHermesDesiredOrders(context.Background(), cfg, plan, []liveguard.ManagedDesiredOrder{desired}, nil, exchange, db, execCtx, false)
	if len(exchange.accepted) != 1 || len(first.Blocked) != 1 {
		t.Fatalf("first Hermes cycle must reach exchange once and block unknown: calls=%d result=%+v", len(exchange.accepted), first)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Status != live.StatusUnknownNeedsManualCheck {
		t.Fatalf("Hermes timeout must remain active unknown after restart: %+v", open)
	}
	second := liveguard.ExecuteHermesDesiredOrders(context.Background(), cfg, plan, []liveguard.ManagedDesiredOrder{desired}, open, exchange, db, execCtx, false)
	if len(exchange.accepted) != 1 || len(second.Blocked) != 1 {
		t.Fatalf("ambiguous Hermes BUY was submitted again: calls=%d open=%+v second=%+v", len(exchange.accepted), open, second)
	}
}

func writeUnknownOutcomeQualityReport(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("reports", 0700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"per_coin": map[string]any{"ETHUSDT": map[string]any{"quality_score": 80, "quality_grade": "A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("reports/live_manager_history_latest.json", b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove("reports/live_manager_history_latest.json") })
}

type acceptedThenTimeoutExchange struct{ accepted []live.LimitOrderRequest }

func (e *acceptedThenTimeoutExchange) PlaceSpotLimitOrder(_ context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	e.accepted = append(e.accepted, req)
	return live.OrderResult{}, context.DeadlineExceeded
}
func (*acceptedThenTimeoutExchange) CancelOrder(_ context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	return live.CancelOrderResult{InstID: req.InstID, OrderID: req.OrderID, ClientOrderID: req.ClientOrderID, Canceled: true}, nil
}
func (*acceptedThenTimeoutExchange) OrderBook(_ context.Context, _ string) (liquidity.OrderBookSnapshot, error) {
	return liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}, nil
}

type alwaysRunningHaltReader struct{}

func (alwaysRunningHaltReader) IsHalted() (bool, error) { return false, nil }

func unknownOutcomeConfig() config.Config {
	var cfg config.Config
	cfg.Live.Enabled, cfg.Live.AutoExecute, cfg.Live.OrderManagementEnabled, cfg.Live.LiveAutoMode = true, true, true, true
	cfg.Live.LiveAutoMaxNotionalUSDT, cfg.Live.MaxOrderNotionalUSDT = 2, 2
	cfg.Live.MaxAutoLayersPerAsset, cfg.Live.MaxOpenLiveOrdersPerAsset, cfg.Live.MaxOpenLiveOrdersTotal = 1, 1, 1
	cfg.Live.MaxLiveNotionalPerOrderUSDT, cfg.Live.MaxLiveNotionalPerAssetUSDT, cfg.Live.MaxLiveNotionalTotalUSDT = 2, 2, 2
	cfg.Live.RequirePostOnly = true
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.NoFutures, cfg.Risk.NoLeverage, cfg.Risk.SpotLimitOnly = true, true, true
	cfg.Portfolio.TotalCapital = 100
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.20}
	cfg.Risk.MaxTotalDeploymentPerCycle, cfg.Risk.MaxSingleAssetDeployment = 0.70, 0.45
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	return cfg
}
func unknownOutcomePlan() agent2.Plan {
	return agent2.Plan{State: agent2.StateActiveLimit, ActionPermission: agent1.Allowed, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, DiscountZone: market.Zone{Low: 90, High: 100}, Invalidation: 88, Reason: "accepted-timeout regression", Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 2}}}}}
}
