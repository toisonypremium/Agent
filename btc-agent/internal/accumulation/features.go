package accumulation

import (
	"math"

	"btc-agent/internal/market"
)

func effortVsResult(c []market.Candle, support market.Zone) bool {
	if len(c) < 22 || !support.Valid() {
		return false
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgVol := avgVolume(c[:len(c)-1], 20)
	avgRange := avgRange(c[:len(c)-1], 20)
	if avgVol <= 0 || avgRange <= 0 || prev.Close <= 0 {
		return false
	}
	volumeHigh := last.Volume >= avgVol*1.5
	result := math.Abs(last.Close-prev.Close) / avgRange
	nearSupport := last.Low <= support.High*1.03 || last.Close <= support.High*1.04
	return volumeHigh && result <= 0.7 && nearSupport && last.Close >= support.Low*0.99
}

func supplyDryUp(c []market.Candle) bool {
	if len(c) < 25 {
		return false
	}
	recentVol := avgVolume(c[len(c)-3:], 3)
	baseVol := avgVolume(c[:len(c)-3], 20)
	recentRange := avgRange(c[len(c)-3:], 3)
	baseRange := avgRange(c[:len(c)-3], 20)
	if baseVol <= 0 || baseRange <= 0 {
		return false
	}
	return recentVol < baseVol*0.85 && recentRange < baseRange*0.85
}

func retestHold(c []market.Candle, support market.Zone) bool {
	if len(c) < 4 || !support.Valid() {
		return false
	}
	start := len(c) - 3
	for _, x := range c[start:] {
		if x.Close < support.Low || x.Low < support.Low*0.97 {
			return false
		}
	}
	return c[len(c)-1].Close >= support.Mid()
}

func fallingKnifeBreakdown(c []market.Candle, support market.Zone) bool {
	if len(c) < 4 || !support.Valid() {
		return false
	}
	last := c[len(c)-1]
	prev := c[len(c)-2]
	avgVol := avgVolume(c[:len(c)-1], 20)
	if last.Close >= support.Low || last.Close >= prev.Close {
		return false
	}
	lowerLow := last.Low < prev.Low
	volumeHigh := avgVol > 0 && last.Volume >= avgVol*1.2
	return lowerLow && volumeHigh && !retestHold(c, support)
}

func avgVolume(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	for _, x := range c[start:] {
		sum += x.Volume
	}
	return sum / float64(n)
}

func avgRange(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	count := 0
	for _, x := range c[start:] {
		r := x.High - x.Low
		if r <= 0 {
			continue
		}
		sum += r
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
