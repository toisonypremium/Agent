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
	ThresholdKeepCurrent     = "KEEP_CURRENT"
	ThresholdCandidateReview = "CANDIDATE_REVIEW"
	ThresholdRejectNoisy     = "REJECT_NOISY"
	ThresholdNeedMoreData    = "NEED_MORE_DATA"
)

type ThresholdCalibrationConfig struct {
	MinWindow1D int   `json:"min_window_1d"`
	HorizonDays []int `json:"horizon_days"`
}

type ThresholdCalibrationResult struct {
	Enabled bool                  `json:"enabled"`
	Rows    []ThresholdProfileRow `json:"rows"`
	Summary string                `json:"summary"`
}

type ThresholdProfile struct {
	Name                  string  `json:"name"`
	TrendArmedThreshold   float64 `json:"trend_armed_threshold"`
	TrendAllowedThreshold float64 `json:"trend_allowed_threshold"`
	FlowPromoteThreshold  float64 `json:"flow_promote_threshold"`
	MinRewardRisk         float64 `json:"min_reward_risk"`
	ResearchNote          string  `json:"research_note"`
}

type ThresholdProfileRow struct {
	Profile       ThresholdProfile          `json:"profile"`
	Windows       int                       `json:"windows"`
	NoTradeCount  int                       `json:"no_trade_count"`
	WatchCount    int                       `json:"watch_count"`
	ArmedCount    int                       `json:"armed_count"`
	AllowedCount  int                       `json:"allowed_count"`
	ArmedRate     float64                   `json:"armed_rate"`
	AllowedRate   float64                   `json:"allowed_rate"`
	Desired       int                       `json:"desired"`
	Filled        int                       `json:"filled"`
	AvgReturn     map[int]float64           `json:"avg_return"`
	WinRate       map[int]float64           `json:"win_rate"`
	WorstDrawdown map[int]float64           `json:"worst_drawdown"`
	TopBlockers   []BTCPermissionBlockerRow `json:"top_blockers,omitempty"`
	Verdict       string                    `json:"verdict"`
}

type thresholdProfileAcc struct {
	counts      map[agent1.Permission]int
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
	blockers    map[string]int
}

func RunThresholdCalibration(cfg config.Config, btc map[string][]market.Candle, auditCfg ThresholdCalibrationConfig) (ThresholdCalibrationResult, error) {
	auditCfg = normalizeThresholdCalibrationConfig(auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return ThresholdCalibrationResult{}, fmt.Errorf("not enough BTC 1d candles for threshold calibration; need %d got %d", need, len(btc1d))
	}
	profiles := thresholdProfiles()
	accs := make([]*thresholdProfileAcc, len(profiles))
	for i := range profiles {
		accs[i] = newThresholdProfileAcc(auditCfg.HorizonDays)
	}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		entry := btc1d[i].Close
		if entry <= 0 {
			continue
		}
		btcWindow := map[string][]market.Candle{"1d": btc1d[:i+1], "4h": btc1d[:i+1], "1w": btc1d[:i+1]}
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		for j, profile := range profiles {
			perm := evaluateThresholdProfile(analysis, profile)
			acc := accs[j]
			acc.counts[perm]++
			if perm != agent1.Allowed {
				for _, blocker := range btcPermissionBlockers(analysis) {
					acc.blockers[blocker]++
				}
			}
			if perm == agent1.Armed || perm == agent1.Allowed {
				for _, h := range auditCfg.HorizonDays {
					ret := (btc1d[i+h].Close - entry) / entry
					dd := worstDrawdown(btc1d[i+1:i+h+1], entry)
					acc.returns[h] += ret
					if ret > 0 {
						acc.wins[h]++
					}
					if !acc.initialized[h] || dd < acc.worstDD[h] {
						acc.worstDD[h] = dd
						acc.initialized[h] = true
					}
				}
			}
		}
	}
	result := ThresholdCalibrationResult{Enabled: true}
	for i, profile := range profiles {
		result.Rows = append(result.Rows, finalizeThresholdProfileRow(profile, accs[i], auditCfg.HorizonDays, len(cfg.Data.Symbols.Assets)))
	}
	for i := range result.Rows {
		result.Rows[i].Verdict = thresholdProfileVerdict(result.Rows[i], result.Rows[0])
	}
	result.Summary = summarizeThresholdCalibration(result.Rows)
	return result, nil
}

func thresholdProfiles() []ThresholdProfile {
	return []ThresholdProfile{
		{Name: "STRICT_CURRENT", TrendArmedThreshold: 45, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.25, MinRewardRisk: 2.0, ResearchNote: "current production thresholds"},
		{Name: "BALANCED_SAFE", TrendArmedThreshold: 42, TrendAllowedThreshold: 58, FlowPromoteThreshold: 0.22, MinRewardRisk: 2.0, ResearchNote: "research-only mild threshold relaxation"},
		{Name: "ARMED_PROBE_LIGHT", TrendArmedThreshold: 40, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.20, MinRewardRisk: 2.0, ResearchNote: "research-only probe candidate density"},
		{Name: "FLOW_RELAXED", TrendArmedThreshold: 45, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.15, MinRewardRisk: 2.0, ResearchNote: "research-only flow promotion sensitivity"},
		{Name: "RR_RELAXED_SMALL_PROBE", TrendArmedThreshold: 42, TrendAllowedThreshold: 60, FlowPromoteThreshold: 0.22, MinRewardRisk: 1.5, ResearchNote: "research-only lower RR for small probe review"},
	}
}

func evaluateThresholdProfile(a agent1.MarketAnalysis, p ThresholdProfile) agent1.Permission {
	if a.MarketRegime == "PANIC_SELLING" || a.RiskLevel == agent1.High || a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High || !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return agent1.NoTrade
	}
	if btcPermissionRRProxy(a) < p.MinRewardRisk {
		return agent1.Watch
	}
	allowedRegime := a.MarketRegime == "ACCUMULATION" || a.MarketRegime == "WEAK_UPTREND" || a.MarketRegime == "RANGE"
	if a.TrendScore >= p.TrendAllowedThreshold && allowedRegime {
		return agent1.Allowed
	}
	if a.TrendScore >= p.TrendArmedThreshold {
		return agent1.Armed
	}
	flowOK := (a.Flow.Bias == flow.BiasAccumulation || a.Flow.Bias == flow.BiasBearTrap) && a.Flow.Score >= p.FlowPromoteThreshold
	if flowOK {
		return agent1.Armed
	}
	return agent1.Watch
}

func normalizeThresholdCalibrationConfig(auditCfg ThresholdCalibrationConfig) ThresholdCalibrationConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{7, 14}
	}
	seen := map[int]bool{}
	out := []int{}
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

func newThresholdProfileAcc(horizons []int) *thresholdProfileAcc {
	return &thresholdProfileAcc{counts: map[agent1.Permission]int{}, returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}, blockers: map[string]int{}}
}

func finalizeThresholdProfileRow(profile ThresholdProfile, acc *thresholdProfileAcc, horizons []int, assetCount int) ThresholdProfileRow {
	row := ThresholdProfileRow{Profile: profile, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if acc == nil {
		return row
	}
	row.NoTradeCount = acc.counts[agent1.NoTrade]
	row.WatchCount = acc.counts[agent1.Watch]
	row.ArmedCount = acc.counts[agent1.Armed]
	row.AllowedCount = acc.counts[agent1.Allowed]
	row.Windows = row.NoTradeCount + row.WatchCount + row.ArmedCount + row.AllowedCount
	if row.Windows > 0 {
		row.ArmedRate = float64(row.ArmedCount) / float64(row.Windows)
		row.AllowedRate = float64(row.AllowedCount) / float64(row.Windows)
	}
	if assetCount <= 0 {
		assetCount = 1
	}
	row.Desired = (row.ArmedCount + row.AllowedCount) * assetCount
	row.Filled = 0
	actionable := row.ArmedCount + row.AllowedCount
	for _, h := range horizons {
		if actionable > 0 {
			row.AvgReturn[h] = acc.returns[h] / float64(actionable)
			row.WinRate[h] = float64(acc.wins[h]) / float64(actionable)
			row.WorstDrawdown[h] = acc.worstDD[h]
		}
	}
	row.TopBlockers = finalizeBTCPermissionBlockers(acc.blockers, row.Windows)
	if len(row.TopBlockers) > 5 {
		row.TopBlockers = row.TopBlockers[:5]
	}
	return row
}

func thresholdProfileVerdict(row, strict ThresholdProfileRow) string {
	if row.Windows == 0 || row.ArmedCount+row.AllowedCount < 5 {
		return ThresholdNeedMoreData
	}
	if row.Profile.Name == "STRICT_CURRENT" {
		return ThresholdKeepCurrent
	}
	if row.WorstDrawdown[7] < strict.WorstDrawdown[7]-0.03 || row.WinRate[7] < strict.WinRate[7]-0.10 {
		return ThresholdRejectNoisy
	}
	if row.ArmedRate+row.AllowedRate > strict.ArmedRate+strict.AllowedRate && row.WinRate[7] >= strict.WinRate[7] {
		return ThresholdCandidateReview
	}
	return ThresholdKeepCurrent
}

func summarizeThresholdCalibration(rows []ThresholdProfileRow) string {
	if len(rows) == 0 {
		return "Threshold calibration produced no rows."
	}
	candidate := "none"
	for _, row := range rows {
		if row.Verdict == ThresholdCandidateReview {
			candidate = row.Profile.Name
			break
		}
	}
	return fmt.Sprintf("Threshold calibration profiles=%d candidate=%s; research-only, no production thresholds changed", len(rows), candidate)
}
