package agent2

import "btc-agent/internal/market"

func FOMO(c []market.Candle, ema20, rsi float64, resistance market.Zone) bool {
	return ClassifyAssetRisk(c, ema20, rsi, resistance).FOMO == ReasonHardBlock
}
