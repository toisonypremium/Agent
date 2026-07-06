package liveguard

import (
	"sort"
	"time"

	"btc-agent/internal/market"
)

func btcHistoryTimeframeWindow(btc map[string][]market.Candle, dailyIndex int) map[string][]market.Candle {
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
	cutoff := historyCandleCutoffTime(btc1d[dailyIndex])
	return map[string][]market.Candle{
		"1d": daily,
		"4h": historyTimeframePrefixOrFallback(btc["4h"], cutoff, daily),
		"1w": historyTimeframePrefixOrFallback(btc["1w"], cutoff, daily),
	}
}

func historyTimeframePrefixOrFallback(candles []market.Candle, cutoff time.Time, fallback []market.Candle) []market.Candle {
	if len(candles) == 0 || cutoff.IsZero() {
		return fallback
	}
	prefix := historyCandlesUntil(candles, cutoff)
	if len(prefix) == 0 {
		return fallback
	}
	return prefix
}

func historyCandlesUntil(candles []market.Candle, cutoff time.Time) []market.Candle {
	if len(candles) == 0 || cutoff.IsZero() {
		return nil
	}
	end := sort.Search(len(candles), func(i int) bool {
		return historyCandleCutoffTime(candles[i]).After(cutoff)
	})
	return candles[:end]
}

func historyCandleCutoffTime(c market.Candle) time.Time {
	if !c.CloseTime.IsZero() {
		return c.CloseTime
	}
	return c.OpenTime
}
