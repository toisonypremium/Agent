package agent2

import "btc-agent/internal/market"

func FOMO(c []market.Candle, ema20, rsi float64, resistance market.Zone) bool {
	if len(c) < 5 {
		return true
	}
	green := 0
	for i := len(c) - 4; i < len(c); i++ {
		if c[i].Close > c[i].Open {
			green++
		}
	}
	price := c[len(c)-1].Close
	if green >= 4 {
		return true
	}
	if ema20 > 0 && price > ema20*1.10 {
		return true
	}
	if rsi > 72 {
		return true
	}
	if resistance.Valid() && price > resistance.Low*0.98 {
		return true
	}
	return false
}
