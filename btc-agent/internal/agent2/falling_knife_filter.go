package agent2

import "btc-agent/internal/market"

func FallingKnife(c []market.Candle) bool {
	if len(c) < 5 {
		return true
	}
	ll := 0
	for i := len(c) - 4; i < len(c); i++ {
		if c[i].Low < c[i-1].Low {
			ll++
		}
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	return ll >= 3 || (last.Close < prev.Low && last.Volume > prev.Volume*1.5)
}
