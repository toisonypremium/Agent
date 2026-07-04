package liveguard

import (
	"math"
	"strings"
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

func TestRunPreflightCanaryScalesDown(t *testing.T) {
	cfg := preflightConfig()
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: true, Canary: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 1.0}}
	got, result := RunPreflight(cfg, candidate, filters)
	if !result.Pass {
		t.Fatalf("expected preflight pass: %+v", result)
	}
	if got.Notional > 2.0 || got.Quantity != 0.02 {
		t.Fatalf("canary scaling failed: %+v", got)
	}
	if !got.Canary || !result.Canary {
		t.Fatalf("canary flag not propagated: got=%v result=%v", got.Canary, result.Canary)
	}
}

func TestRunPreflightCanaryBlocksWhenBelowMinSize(t *testing.T) {
	cfg := preflightConfig()
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 1000, Quantity: 0.01, Notional: 10, PostOnly: true, Canary: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.001, MinSize: 0.005}}
	// Price = 1000, max notional = 2.0 -> scaled Qty = 2.0 / 1000 = 0.002.
	// But MinSize = 0.005. So scaled quantity 0.002 should fail min size check.
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected min size block for scaled canary: %+v", result)
	}
	if !strings.Contains(strings.Join(result.Reasons, " "), "quantity below min_size") {
		t.Fatalf("missing min size blocker: %+v", result.Reasons)
	}
}

func TestRunPreflightCanaryBlocksWhenBelowMinNotional(t *testing.T) {
	cfg := preflightConfig()
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	candidate := CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.1, Notional: 10, PostOnly: true, Canary: true}
	filters := []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 5.0}}
	// Scaled notional is 2.0, but MinNotional is 5.0. Should fail min notional check.
	_, result := RunPreflight(cfg, candidate, filters)
	if result.Pass {
		t.Fatalf("expected min notional block for scaled canary: %+v", result)
	}
	if !strings.Contains(strings.Join(result.Reasons, " "), "notional below min_notional") {
		t.Fatalf("missing min notional blocker: %+v", result.Reasons)
	}
}
