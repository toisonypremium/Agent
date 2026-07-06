package agent2

import "btc-agent/internal/market"

func FallingKnife(c []market.Candle) bool {
	return ClassifyAssetRisk(c, 0, 0, market.Zone{}).FallingKnife == ReasonHardBlock
}
