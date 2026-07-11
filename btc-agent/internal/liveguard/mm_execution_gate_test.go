package liveguard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"btc-agent/internal/config"
	"btc-agent/internal/liquidity"
)

type fakeOrderBookProvider struct {
	book liquidity.OrderBookSnapshot
	err  error
}

func (f fakeOrderBookProvider) OrderBook(ctx context.Context, instID string) (liquidity.OrderBookSnapshot, error) {
	if f.err != nil {
		return liquidity.OrderBookSnapshot{}, f.err
	}
	return f.book, nil
}

func mmGateConfig() config.Config {
	var cfg config.Config
	cfg.Live.LiquidityGateEnabled = true
	cfg.Live.RequireOrderBookLiquidity = true
	cfg.Live.RequirePostOnly = true
	cfg.Live.MaxSpreadBps = 15
	cfg.Live.MaxSlippageBps = 30
	cfg.Live.MinBidDepthToOrderRatio = 20
	return cfg
}

func mmDesired() ManagedDesiredOrder {
	return ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", Side: "BUY", Type: "limit", Price: 99.9, Quantity: 0.02, Notional: 2, PostOnly: true}
}

func TestEvaluateMMExecutionGateDisabledPasses(t *testing.T) {
	var cfg config.Config
	got := EvaluateMMExecutionGate(context.Background(), cfg, mmDesired(), nil)
	if !got.Pass || got.Enabled {
		t.Fatalf("expected disabled pass: %+v", got)
	}
}

func TestEvaluateMMExecutionGateRequiresOrderBook(t *testing.T) {
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), mmDesired(), nil)
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, ";"), "order book unavailable") {
		t.Fatalf("expected order book block: %+v", got)
	}
}

func TestEvaluateMMExecutionGateBlocksWideSpread(t *testing.T) {
	book := liquidity.OrderBookSnapshot{BestBid: 99, BestAsk: 101, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), mmDesired(), fakeOrderBookProvider{book: book})
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, ";"), "spread") {
		t.Fatalf("expected spread block: %+v", got)
	}
}

func TestEvaluateMMExecutionGateBlocksThinBidDepthAndSlippage(t *testing.T) {
	book := liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 10, AskDepth1PctUSDT: 1000}
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), mmDesired(), fakeOrderBookProvider{book: book})
	joined := strings.Join(got.Reasons, ";")
	if got.Pass || !strings.Contains(joined, "bid depth") || !strings.Contains(joined, "slippage") {
		t.Fatalf("expected depth/slippage block: %+v", got)
	}
}

func TestEvaluateMMExecutionGateBlocksSellPressureProxy(t *testing.T) {
	book := liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 4000}
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), mmDesired(), fakeOrderBookProvider{book: book})
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, ";"), "sell-pressure") {
		t.Fatalf("expected sell-pressure block: %+v", got)
	}
}

func TestEvaluateMMExecutionGateBlocksPostOnlyCross(t *testing.T) {
	d := mmDesired()
	d.Price = 100
	book := liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), d, fakeOrderBookProvider{book: book})
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, ";"), "post-only BUY price") {
		t.Fatalf("expected post-only cross block: %+v", got)
	}
}

func TestEvaluateMMExecutionGateHealthyBookPasses(t *testing.T) {
	book := liquidity.OrderBookSnapshot{BestBid: 99.9, BestAsk: 100, BidDepth1PctUSDT: 1000, AskDepth1PctUSDT: 1000}
	got := EvaluateMMExecutionGate(context.Background(), mmGateConfig(), mmDesired(), fakeOrderBookProvider{book: book})
	if !got.Pass || got.SpreadBps <= 0 || got.BidDepthToOrderRatio <= 0 || got.BidShare <= 0 {
		t.Fatalf("expected healthy pass: %+v", got)
	}
}

func TestEvaluateMMExecutionGateOptionalBookErrorPasses(t *testing.T) {
	cfg := mmGateConfig()
	cfg.Live.RequireOrderBookLiquidity = false
	got := EvaluateMMExecutionGate(context.Background(), cfg, mmDesired(), fakeOrderBookProvider{err: fmt.Errorf("network")})
	if !got.Pass || !strings.Contains(strings.Join(got.Reasons, ";"), "optional error") {
		t.Fatalf("expected optional book error pass: %+v", got)
	}
}
