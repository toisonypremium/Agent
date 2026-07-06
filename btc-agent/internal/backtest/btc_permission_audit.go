package backtest

import (
	"fmt"
	"math"
	"sort"
	"strings"

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
	ScoreRows            []BTCPermissionScoreRow               `json:"score_rows"`
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

type BTCPermissionScoreRow struct {
	Permission       agent1.Permission `json:"permission"`
	Count            int               `json:"count"`
	AvgWeeklyTrend   float64           `json:"avg_weekly_trend"`
	AvgDailyTrend    float64           `json:"avg_daily_trend"`
	AvgFourHourTrend float64           `json:"avg_four_hour_trend"`
	AvgTrendScore    float64           `json:"avg_trend_score"`
	AvgFlowScore     float64           `json:"avg_flow_score"`
	AvgRRProxy       float64           `json:"avg_rr_proxy"`
}

type UnlockCondition struct {
	Name    string  `json:"name"`
	Pass    bool    `json:"pass"`
	Current string  `json:"current,omitempty"`
	Target  string  `json:"target,omitempty"`
	Gap     float64 `json:"gap,omitempty"`
	Reason  string  `json:"reason"`
}

type btcPermissionAcc struct {
	count       int
	trendTotal  float64
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
}

type btcPermissionScoreAcc struct {
	count         int
	weeklyTotal   float64
	dailyTotal    float64
	fourHourTotal float64
	trendTotal    float64
	flowTotal     float64
	rrProxyTotal  float64
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
	scoreAcc := map[agent1.Permission]*btcPermissionScoreAcc{}
	for _, perm := range btcPermissionOrder() {
		scoreAcc[perm] = &btcPermissionScoreAcc{}
	}
	regimeCounts := map[string]int{}
	riskCounts := map[string]int{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	windows := 0

	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		btcWindow := btcTimeframeWindow(btc, i)
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
		addBTCPermissionScore(scoreAcc, analysis)
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
		result.ScoreRows = append(result.ScoreRows, finalizeBTCPermissionScoreRow(perm, scoreAcc[perm]))
	}
	result.Blockers = finalizeBTCPermissionBlockers(blockerCounts, windows)
	result.BlockersByPermission = finalizeBTCPermissionBlockersByPermission(blockerByPermissionCounts, permissionCounts)
	result.Summary = summarizeBTCPermissionAudit(result.Rows, result.Blockers, windows)
	return result, nil
}

func addBTCPermissionScore(acc map[agent1.Permission]*btcPermissionScoreAcc, analysis agent1.MarketAnalysis) {
	a := acc[analysis.ActionPermission]
	if a == nil {
		a = &btcPermissionScoreAcc{}
		acc[analysis.ActionPermission] = a
	}
	a.count++
	a.weeklyTotal += analysis.ScoreBreakdown.WeeklyTrend
	a.dailyTotal += analysis.ScoreBreakdown.DailyTrend
	a.fourHourTotal += analysis.ScoreBreakdown.FourHourTrend
	a.trendTotal += analysis.TrendScore
	a.flowTotal += analysis.Flow.Score
	a.rrProxyTotal += btcPermissionRRProxy(analysis)
}

func finalizeBTCPermissionScoreRow(perm agent1.Permission, a *btcPermissionScoreAcc) BTCPermissionScoreRow {
	row := BTCPermissionScoreRow{Permission: perm}
	if a == nil || a.count == 0 {
		return row
	}
	row.Count = a.count
	denom := float64(a.count)
	row.AvgWeeklyTrend = a.weeklyTotal / denom
	row.AvgDailyTrend = a.dailyTotal / denom
	row.AvgFourHourTrend = a.fourHourTotal / denom
	row.AvgTrendScore = a.trendTotal / denom
	row.AvgFlowScore = a.flowTotal / denom
	row.AvgRRProxy = a.rrProxyTotal / denom
	return row
}

func btcPermissionRRProxy(a agent1.MarketAnalysis) float64 {
	if !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return 0
	}
	entry := a.PrimarySupportZone.High
	invalidation := a.PrimarySupportZone.Low * 0.985
	risk := entry - invalidation
	if risk <= 0 {
		return 0
	}
	return (a.ResistanceZone.High - entry) / risk
}

func PermissionUnlockConditions(a agent1.MarketAnalysis) []UnlockCondition {
	out := []UnlockCondition{}
	trendGapArmed := math.Max(0, 45-a.TrendScore)
	out = append(out, UnlockCondition{Name: "TREND_TO_ARMED", Pass: trendGapArmed == 0, Current: fmt.Sprintf("%.1f", a.TrendScore), Target: "45.0", Gap: trendGapArmed, Reason: fmt.Sprintf("trend score cần +%.1f điểm để lên ARMED", trendGapArmed)})
	trendGapAllowed := math.Max(0, 60-a.TrendScore)
	out = append(out, UnlockCondition{Name: "TREND_TO_ALLOWED", Pass: trendGapAllowed == 0, Current: fmt.Sprintf("%.1f", a.TrendScore), Target: "60.0", Gap: trendGapAllowed, Reason: fmt.Sprintf("trend score cần +%.1f điểm để đủ ALLOWED", trendGapAllowed)})
	allowedRegime := a.MarketRegime == "ACCUMULATION" || a.MarketRegime == "WEAK_UPTREND" || a.MarketRegime == "RANGE"
	out = append(out, UnlockCondition{Name: "ALLOWED_REGIME", Pass: allowedRegime, Current: a.MarketRegime, Target: "ACCUMULATION/WEAK_UPTREND/RANGE", Reason: "regime phải là vùng cho phép để ALLOWED"})
	flowPass := (a.Flow.Bias == flow.BiasAccumulation || a.Flow.Bias == flow.BiasBearTrap) && a.Flow.Score >= 0.25
	out = append(out, UnlockCondition{Name: "FLOW_PROMOTE_ARMED", Pass: flowPass, Current: fmt.Sprintf("%s %.2f", a.Flow.Bias, a.Flow.Score), Target: "ACCUMULATION/BEAR_TRAP >=0.25", Gap: math.Max(0, 0.25-a.Flow.Score), Reason: "flow cần accumulation/bear-trap hoặc reclaim/absorption rõ để hỗ trợ ARMED"})
	rr := btcPermissionRRProxy(a)
	out = append(out, UnlockCondition{Name: "RR_PROXY", Pass: rr >= 2, Current: fmt.Sprintf("%.2f", rr), Target: "2.00", Gap: math.Max(0, 2-rr), Reason: "reward/risk proxy BTC cần >=2.00"})
	hard := a.MarketRegime == "PANIC_SELLING" || a.RiskLevel == agent1.High || a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High || !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid()
	out = append(out, UnlockCondition{Name: "HARD_BLOCKERS", Pass: !hard, Current: strings.Join(btcPermissionBlockers(a), ";"), Target: "no panic/high risk/high falling knife/high FOMO/invalid zones", Reason: "hard blockers phải sạch trước khi live order"})
	return out
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
	if missingFlowConfirmationIsActionable(a) {
		if a.Flow.Bias == flow.BiasNeutral {
			out = append(out, BlockerFlowNeutral)
		}
		if a.Flow.Score < 0.25 {
			out = append(out, BlockerFlowWeakScore)
		}
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

func missingFlowConfirmationIsActionable(a agent1.MarketAnalysis) bool {
	if a.ActionPermission != agent1.Watch && a.ActionPermission != agent1.Armed {
		return false
	}
	if a.RiskLevel == agent1.High || a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High {
		return false
	}
	if a.Flow.Bias == flow.BiasDistribution || a.Flow.Bias == flow.BiasBullTrap || a.Flow.Daily.Distribution || a.Flow.Daily.FailedBreakout {
		return false
	}
	if !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return false
	}
	if btcPermissionRRProxy(a) < 2 {
		return false
	}
	return a.TrendScore >= 45
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
