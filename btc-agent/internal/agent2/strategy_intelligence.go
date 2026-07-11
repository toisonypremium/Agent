package agent2

import (
	"fmt"
	"sort"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
)

type UnlockGap struct {
	Gate     string            `json:"gate"`
	Severity SetupGateSeverity `json:"severity"`
	Score    float64           `json:"score"`
	Gap      float64           `json:"gap"`
	Reason   string            `json:"reason"`
	Next     string            `json:"next"`
}

type StrategyIntelligence struct {
	Symbol       string      `json:"symbol"`
	SetupScore   float64     `json:"setup_score"`
	ClosestGate  string      `json:"closest_gate"`
	UnlockGaps   []UnlockGap `json:"unlock_gaps"`
	Summary      string      `json:"summary"`
	ResearchOnly bool        `json:"research_only"`
}

type ResearchSizingSuggestion struct {
	Symbol                  string  `json:"symbol"`
	ExecutableNowUSDT       float64 `json:"executable_now_usdt"`
	SuggestedNotionalUSDT   float64 `json:"suggested_notional_usdt"`
	CapUSDT                 float64 `json:"cap_usdt"`
	PortfolioCapUSDT        float64 `json:"portfolio_cap_usdt"`
	SingleAssetCapUSDT      float64 `json:"single_asset_cap_usdt"`
	LiveCapContextUSDT      float64 `json:"live_cap_context_usdt,omitempty"`
	Score                   float64 `json:"score"`
	Reason                  string  `json:"reason"`
	ResearchOnly            bool    `json:"research_only"`
	OrderAuthorityUnchanged bool    `json:"order_authority_unchanged"`
}

func BuildStrategyIntelligence(cfg config.Config, asset AssetPlan) StrategyIntelligence {
	intel := StrategyIntelligence{Symbol: asset.Symbol, SetupScore: asset.SetupScore, ResearchOnly: true}
	for _, gate := range asset.SetupGates {
		if gate.Pass {
			continue
		}
		intel.UnlockGaps = append(intel.UnlockGaps, UnlockGap{
			Gate:     gate.Name,
			Severity: gate.Severity,
			Score:    gate.Score,
			Gap:      clamp01(1 - gate.Score),
			Reason:   enrichUnlockReason(cfg, asset, gate),
			Next:     gate.Next,
		})
	}
	sort.SliceStable(intel.UnlockGaps, func(i, j int) bool {
		if intel.UnlockGaps[i].Severity != intel.UnlockGaps[j].Severity {
			return intel.UnlockGaps[i].Severity == SetupGateHard
		}
		if intel.UnlockGaps[i].Gap != intel.UnlockGaps[j].Gap {
			return intel.UnlockGaps[i].Gap < intel.UnlockGaps[j].Gap
		}
		return intel.UnlockGaps[i].Gate < intel.UnlockGaps[j].Gate
	})
	if len(intel.UnlockGaps) > 0 {
		intel.ClosestGate = intel.UnlockGaps[0].Gate
	}
	intel.Summary = strategySummary(cfg, asset, intel)
	return intel
}

func BuildResearchSizingSuggestion(cfg config.Config, asset AssetPlan, permission agent1.Permission) ResearchSizingSuggestion {
	portfolioCap := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[strings.ToUpper(asset.Symbol)] * cfg.Risk.MaxTotalDeploymentPerCycle
	if portfolioCap <= 0 {
		portfolioCap = cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[asset.Symbol] * cfg.Risk.MaxTotalDeploymentPerCycle
	}
	singleAssetCap := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment
	capUSDT := positiveMin(portfolioCap, singleAssetCap)
	liveCap := liveSizingCapContext(cfg)
	if cfg.Live.Enabled && liveCap > 0 {
		capUSDT = positiveMin(capUSDT, liveCap)
	}
	score := clamp01(asset.SetupScore)
	if score == 0 && len(asset.SetupGates) > 0 {
		sum := 0.0
		for _, gate := range asset.SetupGates {
			sum += gate.Score
		}
		score = clamp01(sum / float64(len(asset.SetupGates)))
	}
	liqScore := 1.0
	if asset.LiquidityQuality.Enabled {
		liqScore = clamp01(asset.LiquidityQuality.Score / 100)
	}
	btcFactor := 0.0
	if permission == agent1.Allowed {
		btcFactor = 1.0
	} else if permission == agent1.Armed {
		btcFactor = 0.35
	}
	candidate := capUSDT * score * liqScore * btcFactor
	candidate = round2(minFloat(candidate, capUSDT))
	executable := 0.0
	if asset.State == StateActiveLimit && permission == agent1.Allowed {
		executable = candidate
	}
	reason := "research only; no order authority changed"
	if asset.State != StateActiveLimit {
		reason = fmt.Sprintf("research only; no order authority for %s; WATCH/SCOUT/ARMED must not create orders", asset.State)
	} else if permission != agent1.Allowed {
		reason = fmt.Sprintf("research only; BTC permission %s; no executable order authority", permission)
	}
	return ResearchSizingSuggestion{
		Symbol:                  asset.Symbol,
		ExecutableNowUSDT:       executable,
		SuggestedNotionalUSDT:   candidate,
		CapUSDT:                 round2(capUSDT),
		PortfolioCapUSDT:        round2(portfolioCap),
		SingleAssetCapUSDT:      round2(singleAssetCap),
		LiveCapContextUSDT:      round2(liveCap),
		Score:                   round2(score * liqScore * btcFactor),
		Reason:                  reason,
		ResearchOnly:            true,
		OrderAuthorityUnchanged: true,
	}
}

func StrategyIntelligenceLine(cfg config.Config, asset AssetPlan, permission agent1.Permission) string {
	intel := BuildStrategyIntelligence(cfg, asset)
	sizing := BuildResearchSizingSuggestion(cfg, asset, permission)
	parts := []string{intel.Summary}
	if len(intel.UnlockGaps) > 0 {
		gap := intel.UnlockGaps[0]
		parts = append(parts, fmt.Sprintf("closest=%s gap=%.2f next=%s", gap.Gate, gap.Gap, firstNonEmptyMM(gap.Next, gap.Reason)))
	}
	parts = append(parts, fmt.Sprintf("research size: executable_now=%.2f USDT suggested_if_unlocked<=%.2f USDT cap=%.2f USDT (%s)", sizing.ExecutableNowUSDT, sizing.SuggestedNotionalUSDT, sizing.CapUSDT, sizing.Reason))
	return strings.Join(parts, " | ")
}

func enrichUnlockReason(cfg config.Config, asset AssetPlan, gate SetupGateResult) string {
	switch gate.Name {
	case EntryCheckRewardRisk:
		if cfg.Risk.MinRewardRisk > 0 && asset.RewardRisk > 0 && asset.RewardRisk < cfg.Risk.MinRewardRisk {
			return fmt.Sprintf("RR gap %.2f: reward/risk %.2f < %.2f", cfg.Risk.MinRewardRisk-asset.RewardRisk, asset.RewardRisk, cfg.Risk.MinRewardRisk)
		}
	case EntryCheckDiscountZone:
		if asset.DiscountGapPct > 0 {
			return fmt.Sprintf("discount gap %.2f%%: price still above support/discount", asset.DiscountGapPct*100)
		}
	case EntryCheckRotationRank, EntryCheckRotationScore:
		if asset.RotationRank > 0 || asset.RotationScore > 0 {
			return fmt.Sprintf("rotation rank=%d score=%.2f: %s", asset.RotationRank, asset.RotationScore, gate.Reason)
		}
	case EntryCheckAssetFlowEntry, EntryCheckMMAccumulation:
		pieces := []string{fmt.Sprintf("flow/MM score %.2f/%.1f", asset.AssetFlowScore, asset.MMScore)}
		if len(asset.MMMissing) > 0 {
			pieces = append(pieces, "missing "+strings.Join(asset.MMMissing, ", "))
		}
		if gate.Reason != "" {
			pieces = append(pieces, gate.Reason)
		}
		return strings.Join(pieces, "; ")
	case EntryCheckLiquidityQuality:
		if asset.LiquidityQuality.Enabled {
			reason := "liquidity proxy"
			if len(asset.LiquidityQuality.Reasons) > 0 {
				reason = asset.LiquidityQuality.Reasons[0]
			}
			return fmt.Sprintf("liquidity grade=%s score=%.1f: %s", asset.LiquidityQuality.Grade, asset.LiquidityQuality.Score, reason)
		}
	}
	return gate.Reason
}

func strategySummary(cfg config.Config, asset AssetPlan, intel StrategyIntelligence) string {
	if len(intel.UnlockGaps) == 0 {
		return fmt.Sprintf("setup intelligence: %s score %.2f; all setup gates passed research-only", asset.Symbol, asset.SetupScore)
	}
	details := []string{}
	if asset.DiscountGapPct > 0 {
		details = append(details, fmt.Sprintf("discount_gap=%.1f%%", asset.DiscountGapPct*100))
	}
	if cfg.Risk.MinRewardRisk > 0 && asset.RewardRisk > 0 && asset.RewardRisk < cfg.Risk.MinRewardRisk {
		details = append(details, fmt.Sprintf("RR_gap=%.2f", cfg.Risk.MinRewardRisk-asset.RewardRisk))
	}
	if asset.RotationRank > 0 || asset.RotationScore > 0 {
		details = append(details, fmt.Sprintf("rotation=#%d/%.2f", asset.RotationRank, asset.RotationScore))
	}
	if asset.AssetFlowScore > 0 || asset.MMScore > 0 {
		details = append(details, fmt.Sprintf("flow/MM=%.2f/%.1f", asset.AssetFlowScore, asset.MMScore))
	}
	if asset.LiquidityQuality.Enabled {
		details = append(details, fmt.Sprintf("liq=%s %.1f", asset.LiquidityQuality.Grade, asset.LiquidityQuality.Score))
	}
	if len(details) == 0 {
		details = append(details, "deterministic gates pending")
	}
	return fmt.Sprintf("setup intelligence: %s score %.2f closest=%s (%s); research only", asset.Symbol, asset.SetupScore, intel.ClosestGate, strings.Join(details, ", "))
}

func liveSizingCapContext(cfg config.Config) float64 {
	caps := []float64{}
	if config.LiveAutoMode(cfg) && config.LiveAutoMaxNotionalUSDT(cfg) > 0 {
		caps = append(caps, config.LiveAutoMaxNotionalUSDT(cfg))
	}
	if cfg.Live.MaxLiveNotionalPerOrderUSDT > 0 {
		caps = append(caps, cfg.Live.MaxLiveNotionalPerOrderUSDT)
	}
	if cfg.Live.MaxOrderNotionalUSDT > 0 {
		caps = append(caps, cfg.Live.MaxOrderNotionalUSDT)
	}
	if len(caps) == 0 {
		return 0
	}
	out := caps[0]
	for _, cap := range caps[1:] {
		out = minFloat(out, cap)
	}
	return out
}

func positiveMin(values ...float64) float64 {
	out := 0.0
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if out == 0 || v < out {
			out = v
		}
	}
	return out
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
