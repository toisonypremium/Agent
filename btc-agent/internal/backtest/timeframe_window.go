package backtest

import (
	"sort"
	"time"

	"btc-agent/internal/market"
)

func btcTimeframeWindow(btc map[string][]market.Candle, dailyIndex int) map[string][]market.Candle {
	btc1d := btc["1d"]
	if len(btc1d) == 0 {
		return map[string][]market.Candle{"1d": nil, "4h": nil, "1w": nil}
	}
	if dailyIndex < 0 {
		dailyIndex = 0
	}
	if dailyIndex >= len(btc1d) {
		dailyIndex = len(btc1d) - 1
	}
	daily := btc1d[:dailyIndex+1]
	cutoff := candleCutoffTime(btc1d[dailyIndex])
	return map[string][]market.Candle{
		"1d": daily,
		"4h": timeframePrefixOrFallback(btc["4h"], cutoff, daily),
		"1w": timeframePrefixOrFallback(btc["1w"], cutoff, daily),
	}
}

func timeframePrefixOrFallback(candles []market.Candle, cutoff time.Time, fallback []market.Candle) []market.Candle {
	if len(candles) == 0 || cutoff.IsZero() {
		return fallback
	}
	prefix := candlesUntil(candles, cutoff)
	if len(prefix) == 0 {
		return fallback
	}
	return prefix
}

func candlesUntil(candles []market.Candle, cutoff time.Time) []market.Candle {
	if len(candles) == 0 || cutoff.IsZero() {
		return nil
	}
	end := sort.Search(len(candles), func(i int) bool {
		return candleCutoffTime(candles[i]).After(cutoff)
	})
	return candles[:end]
}

func candleCutoffTime(c market.Candle) time.Time {
	if !c.CloseTime.IsZero() {
		return c.CloseTime
	}
	return c.OpenTime
}
