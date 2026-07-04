package agent2

import (
	"fmt"
	"sort"

	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type AssetRotationScore struct {
	Symbol          string    `json:"symbol"`
	Rank            int       `json:"rank"`
	Score           float64   `json:"score"`
	Eligible        bool      `json:"eligible"`
	AssetReturn     float64   `json:"asset_return"`
	BenchmarkReturn float64   `json:"benchmark_return"`
	RelativeReturn  float64   `json:"relative_return"`
	MomentumScore   float64   `json:"momentum_score"`
	RelativeScore   float64   `json:"relative_score"`
	DiscountScore   float64   `json:"discount_score"`
	FlowScore       float64   `json:"flow_score"`
	FlowBias        flow.Bias `json:"flow_bias"`
	Reason          string    `json:"reason"`
}

func RankAssets(cfg config.Config, candles map[string][]market.Candle, benchmark []market.Candle) []AssetRotationScore {
	if len(benchmark) == 0 {
		return nil
	}
	lookback, minRelative, minMomentum := rotationStrengthParams(cfg)
	out := []AssetRotationScore{}
	for _, sym := range cfg.Data.Symbols.Assets {
		c := candles[sym]
		s := AssetRotationScore{Symbol: sym, FlowBias: flow.BiasNeutral, Reason: "rotation score skipped: not enough data"}
		if len(c) <= lookback || len(c) < 60 || len(benchmark) <= lookback {
			out = append(out, s)
			continue
		}
		rs := RelativeStrength(c, benchmark, lookback, minRelative, minMomentum)
		s.AssetReturn = rs.AssetReturn
		s.BenchmarkReturn = rs.BenchmarkReturn
		s.RelativeReturn = rs.RelativeReturn
		s.RelativeScore = relativeComponent(rs.RelativeReturn)
		s.MomentumScore = momentumComponent(rs.AssetReturn)
		price := market.LastClose(c)
		support, _ := market.RangeZone(c, 60)
		s.DiscountScore = discountComponent(price, support)
		sig := flow.Analyze(c, "1d", 60)
		s.FlowBias = sig.FlowBias
		s.FlowScore = flowComponent(sig)
		s.Score = clamp01(s.RelativeScore*0.40 + s.MomentumScore*0.25 + s.DiscountScore*0.20 + s.FlowScore*0.15)
		s.Eligible = true
		s.Reason = fmt.Sprintf("score %.2f: rel %.2f momentum %.2f discount %.2f flow %.2f", s.Score, s.RelativeScore, s.MomentumScore, s.DiscountScore, s.FlowScore)
		if !rs.Pass {
			s.Eligible = false
			s.Reason = rs.Reason
		} else if !support.Valid() {
			s.Eligible = false
			s.Reason = "rotation score skipped: invalid support zone"
		} else if sig.FlowBias == flow.BiasDistribution || sig.FlowBias == flow.BiasBullTrap {
			s.Eligible = false
			s.Reason = fmt.Sprintf("asset flow xấu: %s", sig.FlowBias)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Eligible != out[j].Eligible {
			return out[i].Eligible
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Symbol < out[j].Symbol
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func rotationParams(cfg config.Config) (bool, float64, int) {
	if cfg.Risk.DisableRotationScoreFilter {
		return false, 0, 0
	}
	minScore := cfg.Risk.MinRotationScore
	if minScore <= 0 {
		minScore = 0.55
	}
	maxRank := cfg.Risk.MaxRotationRank
	if maxRank <= 0 {
		maxRank = 2
	}
	return true, minScore, maxRank
}

func rotationStrengthParams(cfg config.Config) (int, float64, float64) {
	lookback := cfg.Risk.RelativeStrengthLookbackDays
	if lookback <= 0 {
		lookback = 14
	}
	minRelative := cfg.Risk.MinRelativeStrength
	if minRelative == 0 {
		minRelative = -0.03
	}
	minMomentum := cfg.Risk.MinAssetMomentum
	if minMomentum == 0 {
		minMomentum = -0.05
	}
	return lookback, minRelative, minMomentum
}

func relativeComponent(relativeReturn float64) float64 {
	return clamp01(0.50 + relativeReturn/0.20)
}

func momentumComponent(assetReturn float64) float64 {
	return clamp01(0.50 + assetReturn/0.20)
}

func discountComponent(price float64, support market.Zone) float64 {
	if price <= 0 || !support.Valid() {
		return 0.30
	}
	if price < support.Low*0.97 {
		return 0.10
	}
	if price <= support.High {
		return 1.00
	}
	if price <= support.High*1.05 {
		premium := (price/support.High - 1) / 0.05
		return clamp01(1.00 - premium*0.40)
	}
	if price <= support.High*1.12 {
		return 0.30
	}
	return 0.10
}

func flowComponent(sig flow.Signal) float64 {
	base := 0.55
	switch sig.FlowBias {
	case flow.BiasAccumulation:
		base = 0.90
	case flow.BiasBearTrap:
		base = 0.85
	case flow.BiasDistribution:
		base = 0.15
	case flow.BiasBullTrap:
		base = 0.05
	}
	return clamp01(base + sig.BullScore*0.15 - sig.BearScore*0.20)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
