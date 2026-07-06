package liquidity

import (
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func liqCandles(n int, close, volume float64) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		out[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), Open: close, High: close * 1.02, Low: close * 0.98, Close: close, Volume: volume}
	}
	return out
}

func TestEvaluateCandleProxyPassesLiquidAsset(t *testing.T) {
	cfg := config.Config{}
	got := EvaluateCandleProxy(cfg, "ETHUSDT", liqCandles(40, 100, 100000), 2)
	if !got.Pass || got.Score <= 0 || got.Grade == GradeD {
		t.Fatalf("expected liquid proxy pass: %+v", got)
	}
}

func TestEvaluateCandleProxyBlocksLowVolume(t *testing.T) {
	cfg := config.Config{}
	got := EvaluateCandleProxy(cfg, "ETHUSDT", liqCandles(40, 100, 1), 2)
	if got.Pass || got.Grade != GradeD || len(got.Reasons) == 0 {
		t.Fatalf("expected low volume block: %+v", got)
	}
}

func TestApplyOrderBookBlocksWideSpreadAndThinDepth(t *testing.T) {
	cfg := config.Config{}
	q := EvaluateCandleProxy(cfg, "ETHUSDT", liqCandles(40, 100, 100000), 2)
	got := ApplyOrderBook(cfg, q, OrderBookSnapshot{BestBid: 99, BestAsk: 101, BidDepth1PctUSDT: 10}, 2)
	if got.Pass || len(got.Reasons) == 0 {
		t.Fatalf("expected orderbook block: %+v", got)
	}
}
