package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	BlockerRegimePanicSelling = "REGIME_PANIC_SELLING"
	BlockerRegimeDowntrend    = "REGIME_DOWNTREND"
	BlockerRegimeDistribution = "REGIME_DISTRIBUTION"
	BlockerRegimeTransition   = "REGIME_TRANSITION"
	BlockerRiskHigh           = "RISK_HIGH"
	BlockerRiskMedium         = "RISK_MEDIUM"
	BlockerFallingKnifeHigh   = "FALLING_KNIFE_HIGH"
	BlockerFallingKnifeMedium = "FALLING_KNIFE_MEDIUM"
	BlockerFOMOHigh           = "FOMO_HIGH"
	BlockerFOMOMedium         = "FOMO_MEDIUM"
	BlockerFlowDistribution   = "FLOW_DISTRIBUTION"
	BlockerFlowBullTrap       = "FLOW_BULL_TRAP"
	BlockerFlowNeutral        = "FLOW_NEUTRAL"
	BlockerFlowWeakScore      = "FLOW_WEAK_SCORE"
	BlockerTrendBelow45       = "TREND_BELOW_45"
	BlockerTrendBelow60       = "TREND_BELOW_60"
	BlockerSupportInvalid     = "SUPPORT_INVALID"
	BlockerResistanceInvalid  = "RESISTANCE_INVALID"
	BlockerRRBelow2           = "RR_BELOW_2"
)

type BTCPermissionAuditConfig struct {
	MinWindow1D int   `json:"min_window_1d"`
	HorizonDays []int `json:"horizon_days"`
}

type BTCPermissionAuditResult struct {
	Enabled              bool                                  `json:"enabled"`
	Rows                 []BTCPermissionAuditRow               `json:"rows"`
	Blockers             []BTCPermissionBlockerRow             `json:"blockers"`
	BlockersByPermission []BTCPermissionBlockerByPermissionRow `json:"blockers_by_permission"`
	RegimeCounts         map[string]int                        `json:"regime_counts"`
	RiskCounts           map[string]int                        `json:"risk_counts"`
	Summary              string                                `json:"summary"`
}

type BTCPermissionAuditRow struct {
	Permission    agent1.Permission `json:"permission"`
	Count         int               `json:"count"`
	Rate          float64           `json:"rate"`
	AvgTrendScore float64           `json:"avg_trend_score"`
	AvgReturn     map[int]float64   `json:"avg_return"`
	WinRate       map[int]float64   `json:"win_rate"`
	WorstDrawdown map[int]float64   `json:"worst_drawdown"`
}

type BTCPermissionBlockerRow struct {
	Blocker string  `json:"blocker"`
	Count   int     `json:"count"`
	Rate    float64 `json:"rate"`
}

type BTCPermissionBlockerByPermissionRow struct {
	Permission           agent1.Permission `json:"permission"`
	Blocker              string            `json:"blocker"`
	Count                int               `json:"count"`
	RateWithinPermission float64           `json:"rate_within_permission"`
}

type btcPermissionAcc struct {
	count       int
	trendTotal  float64
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
}

func RunBTCPermissionAudit(cfg config.Config, btc map[string][]market.Candle, auditCfg BTCPermissionAuditConfig) (BTCPermissionAuditResult, error) {
	auditCfg = normalizeBTCPermissionAuditConfig(auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return BTCPermissionAuditResult{}, fmt.Errorf("not enough BTC 1d candles for BTC permission audit; need %d got %d", need, len(btc1d))
	}

	acc := map[agent1.Permission]*btcPermissionAcc{}
	for _, perm := range btcPermissionOrder() {
		acc[perm] = newBTCPermissionAcc(auditCfg.HorizonDays)
	}
	blockerCounts := map[string]int{}
	blockerByPermissionCounts := map[agent1.Permission]map[string]int{}
	permissionCounts := map[agent1.Permission]int{}
	regimeCounts := map[string]int{}
	riskCounts := map[string]int{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	windows := 0

	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		btcWindow := map[string][]market.Candle{"1d": btc1d[:i+1], "4h": btc1d[:i+1], "1w": btc1d[:i+1]}
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		entry := btc1d[i].Close
		if entry <= 0 {
			continue
		}
		windows++
		regimeCounts[analysis.MarketRegime]++
		riskCounts[string(analysis.RiskLevel)]++

		a := acc[analysis.ActionPermission]
		if a == nil {
			a = newBTCPermissionAcc(auditCfg.HorizonDays)
			acc[analysis.ActionPermission] = a
		}
		a.count++
		permissionCounts[analysis.ActionPermission]++
		a.trendTotal += analysis.TrendScore
		for _, h := range auditCfg.HorizonDays {
			future := btc1d[i+h]
			ret := (future.Close - entry) / entry
			dd := worstDrawdown(btc1d[i+1:i+h+1], entry)
			a.returns[h] += ret
			if ret > 0 {
				a.wins[h]++
			}
			if !a.initialized[h] || dd < a.worstDD[h] {
				a.worstDD[h] = dd
				a.initialized[h] = true
			}
		}
		if analysis.ActionPermission != agent1.Allowed {
			for _, blocker := range btcPermissionBlockers(analysis) {
				blockerCounts[blocker]++
				if blockerByPermissionCounts[analysis.ActionPermission] == nil {
					blockerByPermissionCounts[analysis.ActionPermission] = map[string]int{}
				}
				blockerByPermissionCounts[analysis.ActionPermission][blocker]++
			}
		}
	}

	result := BTCPermissionAuditResult{Enabled: true, RegimeCounts: regimeCounts, RiskCounts: riskCounts}
	for _, perm := range btcPermissionOrder() {
		result.Rows = append(result.Rows, finalizeBTCPermissionRow(perm, acc[perm], auditCfg.HorizonDays, windows))
	}
	result.Blockers = finalizeBTCPermissionBlockers(blockerCounts, windows)
	result.BlockersByPermission = finalizeBTCPermissionBlockersByPermission(blockerByPermissionCounts, permissionCounts)
	result.Summary = summarizeBTCPermissionAudit(result.Rows, result.Blockers, windows)
	return result, nil
}

func normalizeBTCPermissionAuditConfig(auditCfg BTCPermissionAuditConfig) BTCPermissionAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	out := auditCfg.HorizonDays[:0]
	seen := map[int]bool{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	sort.Ints(out)
	auditCfg.HorizonDays = out
	return auditCfg
}

func btcPermissionOrder() []agent1.Permission {
	return []agent1.Permission{agent1.Allowed, agent1.Armed, agent1.Watch, agent1.NoTrade}
}

func newBTCPermissionAcc(horizons []int) *btcPermissionAcc {
	a := &btcPermissionAcc{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func finalizeBTCPermissionRow(perm agent1.Permission, a *btcPermissionAcc, horizons []int, total int) BTCPermissionAuditRow {
	row := BTCPermissionAuditRow{Permission: perm, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if a == nil {
		return row
	}
	row.Count = a.count
	if total > 0 {
		row.Rate = float64(a.count) / float64(total)
	}
	if a.count > 0 {
		row.AvgTrendScore = a.trendTotal / float64(a.count)
	}
	for _, h := range horizons {
		if a.count > 0 {
			row.AvgReturn[h] = a.returns[h] / float64(a.count)
			row.WinRate[h] = float64(a.wins[h]) / float64(a.count)
			row.WorstDrawdown[h] = a.worstDD[h]
		}
	}
	return row
}

func btcPermissionBlockers(a agent1.MarketAnalysis) []string {
	out := []string{}
	switch a.MarketRegime {
	case "PANIC_SELLING":
		out = append(out, BlockerRegimePanicSelling)
	case "DOWNTREND":
		out = append(out, BlockerRegimeDowntrend)
	case "DISTRIBUTION":
		out = append(out, BlockerRegimeDistribution)
	case "TRANSITION":
		out = append(out, BlockerRegimeTransition)
	}
	switch a.RiskLevel {
	case agent1.High:
		out = append(out, BlockerRiskHigh)
	case agent1.Medium:
		out = append(out, BlockerRiskMedium)
	}
	switch a.FallingKnifeRisk {
	case agent1.High:
		out = append(out, BlockerFallingKnifeHigh)
	case agent1.Medium:
		out = append(out, BlockerFallingKnifeMedium)
	}
	switch a.FomoRisk {
	case agent1.High:
		out = append(out, BlockerFOMOHigh)
	case agent1.Medium:
		out = append(out, BlockerFOMOMedium)
	}
	if a.Flow.Bias == flow.BiasDistribution || a.Flow.Daily.Distribution {
		out = append(out, BlockerFlowDistribution)
	}
	if a.Flow.Bias == flow.BiasBullTrap || a.Flow.Daily.FailedBreakout {
		out = append(out, BlockerFlowBullTrap)
	}
	if a.Flow.Bias == flow.BiasNeutral {
		out = append(out, BlockerFlowNeutral)
	}
	if a.Flow.Score < 0.25 {
		out = append(out, BlockerFlowWeakScore)
	}
	if a.TrendScore < 45 {
		out = append(out, BlockerTrendBelow45)
	} else if a.TrendScore < 60 {
		out = append(out, BlockerTrendBelow60)
	}
	if !a.PrimarySupportZone.Valid() {
		out = append(out, BlockerSupportInvalid)
	}
	if !a.ResistanceZone.Valid() {
		out = append(out, BlockerResistanceInvalid)
	}
	if a.PrimarySupportZone.Valid() && a.ResistanceZone.Valid() {
		riskWidth := a.PrimarySupportZone.High - a.PrimarySupportZone.Low
		if riskWidth <= 0 || (a.ResistanceZone.High-a.PrimarySupportZone.High)/riskWidth < 2 {
			out = append(out, BlockerRRBelow2)
		}
	}
	return uniqueBlockers(out)
}

func uniqueBlockers(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func finalizeBTCPermissionBlockers(counts map[string]int, total int) []BTCPermissionBlockerRow {
	rows := []BTCPermissionBlockerRow{}
	for blocker, count := range counts {
		row := BTCPermissionBlockerRow{Blocker: blocker, Count: count}
		if total > 0 {
			row.Rate = float64(count) / float64(total)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Blocker < rows[j].Blocker
	})
	return rows
}

func finalizeBTCPermissionBlockersByPermission(counts map[agent1.Permission]map[string]int, permissionCounts map[agent1.Permission]int) []BTCPermissionBlockerByPermissionRow {
	rows := []BTCPermissionBlockerByPermissionRow{}
	for _, perm := range btcPermissionOrder() {
		total := permissionCounts[perm]
		for blocker, count := range counts[perm] {
			row := BTCPermissionBlockerByPermissionRow{Permission: perm, Blocker: blocker, Count: count}
			if total > 0 {
				row.RateWithinPermission = float64(count) / float64(total)
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Permission != rows[j].Permission {
			return permissionOrderIndex(rows[i].Permission) < permissionOrderIndex(rows[j].Permission)
		}
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Blocker < rows[j].Blocker
	})
	return rows
}

func permissionOrderIndex(perm agent1.Permission) int {
	for i, item := range btcPermissionOrder() {
		if item == perm {
			return i
		}
	}
	return len(btcPermissionOrder())
}

func summarizeBTCPermissionAudit(rows []BTCPermissionAuditRow, blockers []BTCPermissionBlockerRow, windows int) string {
	if windows == 0 {
		return "BTC permission audit produced no windows; not enough BTC candles."
	}
	byPerm := map[agent1.Permission]BTCPermissionAuditRow{}
	for _, row := range rows {
		byPerm[row.Permission] = row
	}
	topBlocker := "none"
	if len(blockers) > 0 {
		topBlocker = blockers[0].Blocker
	}
	return fmt.Sprintf("BTC permission audit windows=%d allowed=%d(%.1f%%) armed=%d(%.1f%%) watch=%d(%.1f%%) no_trade=%d(%.1f%%) top_blocker=%s", windows, byPerm[agent1.Allowed].Count, byPerm[agent1.Allowed].Rate*100, byPerm[agent1.Armed].Count, byPerm[agent1.Armed].Rate*100, byPerm[agent1.Watch].Count, byPerm[agent1.Watch].Rate*100, byPerm[agent1.NoTrade].Count, byPerm[agent1.NoTrade].Rate*100, topBlocker)
}
