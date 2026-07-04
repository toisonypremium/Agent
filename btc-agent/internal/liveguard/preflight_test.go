package liveguard

import (
	"math"
	"testing"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

func TestRunPreflightPassRoundsDown(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100.019, Quantity: 0.123456, Notional: 12.35, PostOnly: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 5}}
	got, result := RunPreflight(cfg, candidate, filters)
	if !result.Pass {
		t.Fatalf("preflight failed: %+v", result)
	}
	if !near(got.Price, 100.01) || !near(got.Quantity, 0.1234) {
		t.Fatalf("not rounded down: %+v", got)
	}
}

func TestRunPreflightMissingFilterBlocks(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: true}
	_, result := RunPreflight(cfg, candidate, nil)
	if result.Pass || len(result.Reasons) == 0 {
		t.Fatalf("expected block: %+v", result)
	}
}

func TestRunPreflightMinSizeBlocks(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.0009, Notional: 0.09, PostOnly: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001}}
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected min size block: %+v", result)
	}
}

func TestRunPreflightMinNotionalBlocks(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.01, Notional: 1, PostOnly: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 5}}
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected min notional block: %+v", result)
	}
}

func TestRunPreflightNotionalCapBlocks(t *testing.T) {
	cfg := preflightConfig()
	cfg.Live.MaxOrderNotionalUSDT = 5
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001}}
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected cap block: %+v", result)
	}
}

func TestRunPreflightMarketOrderBlocks(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "market", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001}}
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected market block: %+v", result)
	}
}

func TestRunPreflightPostOnlyBlocks(t *testing.T) {
	cfg := preflightConfig()
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: false}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001}}
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected post-only block: %+v", result)
	}
}

func preflightConfig() config.Config {
	var cfg config.Config
	cfg.Live.RequirePostOnly = true
	cfg.Live.MaxOrderNotionalUSDT = 20
	return cfg
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
