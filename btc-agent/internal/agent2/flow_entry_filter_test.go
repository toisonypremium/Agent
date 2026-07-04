package agent2

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/market"
)

func entryFlowCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*7)%40)
		out[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 1, Volume: 1000}
	}
	return out
}

func TestAssetFlowEntryAllowsReclaimAbsorption(t *testing.T) {
	c := entryFlowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 100, High: 108, Low: 96, Close: 106, Volume: 2200}
	got := AssetFlowEntry("ETHUSDT", c, 0.25, true)
	if !got.Pass || got.HardBlock {
		t.Fatalf("expected flow entry pass: %+v", got)
	}
	if !strings.Contains(got.Reason, "OK") {
		t.Fatalf("expected OK reason: %+v", got)
	}
}

func TestAssetFlowEntryBlocksDistribution(t *testing.T) {
	c := entryFlowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 137, High: 148, Low: 132, Close: 134, Volume: 2300}
	got := AssetFlowEntry("ETHUSDT", c, 0.25, true)
	if got.Pass || !got.HardBlock {
		t.Fatalf("expected hard block: %+v", got)
	}
	if !strings.Contains(got.Reason, "chặn") {
		t.Fatalf("expected block reason: %+v", got)
	}
}

func TestAssetFlowEntryWatchesNeutral(t *testing.T) {
	c := entryFlowCandles(80)
	last := len(c) - 1
	c[last] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", Open: 104, High: 106, Low: 101, Close: 105, Volume: 1000}
	got := AssetFlowEntry("ETHUSDT", c, 0.25, true)
	if got.Pass || got.HardBlock {
		t.Fatalf("expected neutral watch: %+v", got)
	}
	if !strings.Contains(got.Reason, "chưa xác nhận") {
		t.Fatalf("expected watch reason: %+v", got)
	}
}
