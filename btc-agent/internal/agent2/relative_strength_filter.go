package agent2

import (
	"fmt"

	"btc-agent/internal/market"
)

type RelativeStrengthSignal struct {
	Lookback        int     `json:"lookback"`
	AssetReturn     float64 `json:"asset_return"`
	BenchmarkReturn float64 `json:"benchmark_return"`
	RelativeReturn  float64 `json:"relative_return"`
	Pass            bool    `json:"pass"`
	Reason          string  `json:"reason"`
}

func RelativeStrength(asset, benchmark []market.Candle, lookback int, minRelative, minAssetMomentum float64) RelativeStrengthSignal {
	s := RelativeStrengthSignal{Lookback: lookback, Pass: true, Reason: "relative strength skipped: not enough data"}
	if lookback <= 0 {
		lookback = 14
		s.Lookback = lookback
	}
	if len(asset) <= lookback || len(benchmark) <= lookback {
		return s
	}
	assetStart := asset[len(asset)-1-lookback].Close
	assetEnd := asset[len(asset)-1].Close
	benchStart := benchmark[len(benchmark)-1-lookback].Close
	benchEnd := benchmark[len(benchmark)-1].Close
	if assetStart <= 0 || benchStart <= 0 {
		return s
	}
	s.AssetReturn = (assetEnd - assetStart) / assetStart
	s.BenchmarkReturn = (benchEnd - benchStart) / benchStart
	s.RelativeReturn = s.AssetReturn - s.BenchmarkReturn
	if s.RelativeReturn < minRelative && s.AssetReturn < minAssetMomentum {
		s.Pass = false
		s.Reason = fmt.Sprintf("relative strength filter chặn asset: asset=%.2f%% btc=%.2f%% rel=%.2f%%", s.AssetReturn*100, s.BenchmarkReturn*100, s.RelativeReturn*100)
		return s
	}
	s.Pass = true
	s.Reason = fmt.Sprintf("relative strength OK: asset=%.2f%% btc=%.2f%% rel=%.2f%%", s.AssetReturn*100, s.BenchmarkReturn*100, s.RelativeReturn*100)
	return s
}
