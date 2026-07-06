package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

type ZoneEntrySanityResult struct {
	Enabled     bool                     `json:"enabled"`
	Rows        []ZoneEntrySanityRow     `json:"rows"`
	Summary     string                   `json:"summary"`
	TopBlockers []ZoneEntrySanityBlocker `json:"top_blockers,omitempty"`
}

type ZoneEntrySanityRow struct {
	Symbol           string  `json:"symbol"`
	Samples          int     `json:"samples"`
	DiscountPass     int     `json:"discount_pass"`
	RewardRiskPass   int     `json:"reward_risk_pass"`
	ZoneWarn         int     `json:"zone_warn"`
	DiscountPassRate float64 `json:"discount_pass_rate"`
	RewardRiskRate   float64 `json:"reward_risk_rate"`
	AvgZoneWidthPct  float64 `json:"avg_zone_width_pct"`
	AvgDiscountGap   float64 `json:"avg_discount_gap"`
}

type ZoneEntrySanityBlocker struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func RunZoneEntrySanity(cfg config.Config, assets map[string][]market.Candle) ZoneEntrySanityResult {
	result := ZoneEntrySanityResult{Enabled: true}
	blockers := map[string]int{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		candles := assets[symbol]
		row := ZoneEntrySanityRow{Symbol: symbol}
		for i := 60; i < len(candles); i++ {
			window := map[string][]market.Candle{symbol: candles[:i+1]}
			plan := agent2.BuildPlanWithBenchmarks(cfg, dummyAllowedAnalysis(), window, nil)
			if len(plan.Assets) == 0 {
				continue
			}
			asset := plan.Assets[0]
			row.Samples++
			row.AvgZoneWidthPct += asset.ZoneWidthPct
			row.AvgDiscountGap += asset.DiscountGapPct
			if asset.ZoneQuality != "" && asset.ZoneQuality != "ZONE_OK" {
				row.ZoneWarn++
			}
			premium := cfg.Risk.DiscountZonePremiumPct
			if premium <= 0 {
				premium = 0.05
			}
			if asset.DiscountZone.Valid() && asset.DiscountGapPct <= premium {
				row.DiscountPass++
			}
			if asset.RewardRisk >= cfg.Risk.MinRewardRisk {
				row.RewardRiskPass++
			}
			if asset.State != agent2.StateActiveLimit && asset.Reason != "" {
				blockers[asset.Reason]++
			}
		}
		if row.Samples > 0 {
			row.DiscountPassRate = float64(row.DiscountPass) / float64(row.Samples)
			row.RewardRiskRate = float64(row.RewardRiskPass) / float64(row.Samples)
			row.AvgZoneWidthPct /= float64(row.Samples)
			row.AvgDiscountGap /= float64(row.Samples)
		}
		result.Rows = append(result.Rows, row)
	}
	result.TopBlockers = topZoneEntryBlockers(blockers, 8)
	result.Summary = summarizeZoneEntrySanity(result.Rows)
	return result
}

func dummyAllowedAnalysis() agent1.MarketAnalysis {
	return agent1.MarketAnalysis{ActionPermission: agent1.Allowed, MarketRegime: "RANGE", RiskLevel: agent1.Low, FallingKnifeRisk: agent1.Low, FomoRisk: agent1.Low}
}

func summarizeZoneEntrySanity(rows []ZoneEntrySanityRow) string {
	if len(rows) == 0 {
		return "Zone entry sanity rows=0"
	}
	samples, discount, rr, warnings := 0, 0, 0, 0
	for _, row := range rows {
		samples += row.Samples
		discount += row.DiscountPass
		rr += row.RewardRiskPass
		warnings += row.ZoneWarn
	}
	if samples == 0 {
		return "Zone entry sanity samples=0"
	}
	return fmt.Sprintf("Zone entry sanity samples=%d discount_pass=%.1f%% rr_pass=%.1f%% zone_warn=%d", samples, float64(discount)/float64(samples)*100, float64(rr)/float64(samples)*100, warnings)
}

func topZoneEntryBlockers(counts map[string]int, limit int) []ZoneEntrySanityBlocker {
	items := []ZoneEntrySanityBlocker{}
	for reason, count := range counts {
		items = append(items, ZoneEntrySanityBlocker{Reason: reason, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Reason < items[j].Reason
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}
