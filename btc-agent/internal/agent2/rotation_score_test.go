package agent2

import (
	"testing"
	"time"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func scoreCandles(symbol string, start, end float64, n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		progress := 0.0
		if n > 1 {
			progress = float64(i) / float64(n-1)
		}
		wave := float64((i%6)-3) * 0.35
		price := start + (end-start)*progress + wave
		open := price * 1.002
		if i%2 == 0 {
			open = price * 0.998
		}
		out[i] = market.Candle{Symbol: symbol, Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: open, High: price * 1.015, Low: price * 0.985, Close: price, Volume: 1000}
	}
	return out
}

func TestRankAssetsRanksStrongerAssetFirst(t *testing.T) {
	cfg := testConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	btc := scoreCandles("BTCUSDT", 100, 101, 80)
	eth := scoreCandles("ETHUSDT", 100, 100, 80)
	sol := scoreCandles("SOLUSDT", 100, 100, 80)
	for i := len(eth) - 15; i < len(eth); i++ {
		step := float64(i - (len(eth) - 15))
		eth[i].Close = 96 + step*0.30
		eth[i].Open = eth[i].Close * 1.002
		eth[i].High = eth[i].Close * 1.015
		eth[i].Low = eth[i].Close * 0.985
		sol[i].Close = 104 - step*0.30
		sol[i].Open = sol[i].Close * 1.002
		sol[i].High = sol[i].Close * 1.015
		sol[i].Low = sol[i].Close * 0.985
	}
	assets := map[string][]market.Candle{"ETHUSDT": eth, "SOLUSDT": sol}
	got := RankAssets(cfg, assets, btc)
	if len(got) != 2 {
		t.Fatalf("expected 2 scores: %+v", got)
	}
	if got[0].Symbol != "ETHUSDT" || got[0].Rank != 1 {
		t.Fatalf("expected ETH rank 1: %+v", got)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("stronger asset should score higher: %+v", got)
	}
}

func TestRankAssetsPenalizesDistributionFlow(t *testing.T) {
	bad := flowComponent(flow.Signal{FlowBias: flow.BiasDistribution, BearScore: 0.60})
	neutral := flowComponent(flow.Signal{FlowBias: flow.BiasNeutral})
	good := flowComponent(flow.Signal{FlowBias: flow.BiasAccumulation, BullScore: 0.40})
	if !(bad < neutral && neutral < good) {
		t.Fatalf("unexpected flow scores bad=%v neutral=%v good=%v", bad, neutral, good)
	}
}

func TestRankAssetsRequiresBenchmark(t *testing.T) {
	cfg := testConfig()
	got := RankAssets(cfg, map[string][]market.Candle{"ETHUSDT": scoreCandles("ETHUSDT", 100, 110, 80)}, nil)
	if len(got) != 0 {
		t.Fatalf("expected no ranking without benchmark: %+v", got)
	}
}
