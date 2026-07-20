package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
	"btc-agent/internal/researchprofile"
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
	Enabled         bool                      `json:"enabled"`
	Rows            []ThresholdProfileRow     `json:"rows"`
	ProfileRows     []ThresholdProfileRow     `json:"profile_rows"`
	CalibrationRows []ThresholdProfileRow     `json:"calibration_rows"`
	ValidationRows  []ThresholdProfileRow     `json:"validation_rows"`
	SelectedProfile string                    `json:"selected_profile"`
	Split           ThresholdCalibrationSplit `json:"split"`
	Summary         string                    `json:"summary"`
}
type ThresholdCalibrationSplit struct{ ProfileEnd, CalibrationStart, CalibrationEnd, ValidationStart, ValidationEnd, Embargo int }

type ThresholdProfile = researchprofile.Profile

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
	need := auditCfg.MinWindow1D + maxH*2 + 30
	if len(btc1d) < need {
		return ThresholdCalibrationResult{}, fmt.Errorf("not enough BTC 1d candles for purged threshold calibration; need %d got %d", need, len(btc1d))
	}
	available := len(btc1d) - auditCfg.MinWindow1D - 2*maxH
	profileN := available * 60 / 100
	calibrationN := available * 20 / 100
	profileEnd := auditCfg.MinWindow1D + profileN
	calibrationStart := profileEnd + maxH
	calibrationEnd := calibrationStart + calibrationN
	validationStart := calibrationEnd + maxH
	if validationStart+maxH >= len(btc1d) {
		return ThresholdCalibrationResult{}, fmt.Errorf("insufficient purged validation window")
	}
	profiles := researchprofile.Profiles()
	ranges := [][2]int{{auditCfg.MinWindow1D, profileEnd}, {calibrationStart, calibrationEnd}, {validationStart, len(btc1d) - maxH}}
	all := make([][]ThresholdProfileRow, 3)
	for r, bounds := range ranges {
		accs := make([]*thresholdProfileAcc, len(profiles))
		for i := range accs {
			accs[i] = newThresholdProfileAcc(auditCfg.HorizonDays)
		}
		accumulateThresholdRange(cfg, btc, auditCfg, profiles, accs, bounds[0], bounds[1])
		for i, p := range profiles {
			all[r] = append(all[r], finalizeThresholdProfileRow(p, accs[i], auditCfg.HorizonDays, len(cfg.Data.Symbols.Assets)))
		}
	}
	selected := 0
	for i := range all[1] {
		all[1][i].Verdict = thresholdProfileVerdict(all[1][i], all[1][0])
		if all[1][i].Verdict == ThresholdCandidateReview && selected == 0 {
			selected = i
		}
	}
	for i := range all[2] {
		if i == 0 {
			all[2][i].Verdict = ThresholdKeepCurrent
		} else if i == selected {
			all[2][i].Verdict = thresholdProfileVerdict(all[2][i], all[2][0])
		} else {
			all[2][i].Verdict = ThresholdNeedMoreData
		}
	}
	result := ThresholdCalibrationResult{Enabled: true, Rows: all[2], ProfileRows: all[0], CalibrationRows: all[1], ValidationRows: all[2], SelectedProfile: profiles[selected].Name, Split: ThresholdCalibrationSplit{profileEnd, calibrationStart, calibrationEnd, validationStart, len(btc1d) - maxH, maxH}}
	result.Summary = fmt.Sprintf("purged 60/20/20 threshold calibration selected=%s embargo=%d; validation-only verdict, research-only", result.SelectedProfile, maxH)
	return result, nil
}
func accumulateThresholdRange(cfg config.Config, btc map[string][]market.Candle, auditCfg ThresholdCalibrationConfig, profiles []ThresholdProfile, accs []*thresholdProfileAcc, start, end int) {
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := start; i < end; i++ {
		entry := btc["1d"][i].Close
		if entry <= 0 {
			continue
		}
		analysis, err := agent1.Analyze(cfg, btcTimeframeWindow(btc, i), neutralFG)
		if err != nil {
			continue
		}
		for j, profile := range profiles {
			perm := researchprofile.EvaluatePermission(analysis, profile)
			acc := accs[j]
			acc.counts[perm]++
			if perm != agent1.Allowed {
				for _, blocker := range btcPermissionBlockers(analysis) {
					acc.blockers[blocker]++
				}
			}
			if perm == agent1.Armed || perm == agent1.Allowed {
				for _, h := range auditCfg.HorizonDays {
					ret := (btc["1d"][i+h].Close - entry) / entry
					dd := worstDrawdown(btc["1d"][i+1:i+h+1], entry)
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
