package backtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type Config struct {
	MinWindow1D int
	HorizonDays []int
}

type SignalStats struct {
	Count         int             `json:"count"`
	AvgReturn     map[int]float64 `json:"avg_return"`
	WinRate       map[int]float64 `json:"win_rate"`
	WorstDrawdown map[int]float64 `json:"worst_drawdown"`
}

type Result struct {
	GeneratedAt                   time.Time                     `json:"generated_at"`
	Symbol                        string                        `json:"symbol"`
	PeriodStart                   time.Time                     `json:"period_start"`
	PeriodEnd                     time.Time                     `json:"period_end"`
	WindowsTested                 int                           `json:"windows_tested"`
	Horizons                      []int                         `json:"horizons"`
	FlowParams                    flow.Params                   `json:"flow_params"`
	SignalDensity                 float64                       `json:"signal_density"`
	FlowCounts                    map[flow.Bias]int             `json:"flow_counts"`
	ByBias                        map[flow.Bias]SignalStats     `json:"by_bias"`
	BTCFlowBottleneckAudit        BTCFlowBottleneckAuditResult  `json:"btc_flow_bottleneck_audit"`
	FlowParamQualityAudit         FlowParamQualityAuditResult   `json:"flow_param_quality_audit"`
	BTCFlowRegimeAudit            BTCFlowRegimeAuditResult      `json:"btc_flow_regime_audit"`
	BTCPermissionAudit            BTCPermissionAuditResult      `json:"btc_permission_audit"`
	Agent2Simulation              Agent2Simulation              `json:"agent2_simulation"`
	Agent2ArmedResearchSimulation Agent2Simulation              `json:"agent2_armed_research_simulation"`
	WatchlistTriggerAudit         WatchlistTriggerAuditResult   `json:"watchlist_trigger_audit"`
	NearMissWatchlistAudit        WatchlistTriggerAuditResult   `json:"near_miss_watchlist_audit"`
	AssetFlowEntryAudit           AssetFlowEntryAuditResult     `json:"asset_flow_entry_audit"`
	NearMissLayerAudit            NearMissLayerAuditResult      `json:"near_miss_layer_audit"`
	ChecklistPassCountAudit       ChecklistPassCountAuditResult `json:"checklist_pass_count_audit"`
	LayerAudit                    LayerAuditResult              `json:"layer_audit"`
	ExitAudit                     ExitAuditResult               `json:"exit_audit"`
	Summary                       string                        `json:"summary"`
}

type accStats struct {
	count       int
	returns     map[int]float64
	wins        map[int]int
	worstDD     map[int]float64
	initialized map[int]bool
}

func RunBTC(cfg Config, daily []market.Candle) (Result, error) {
	cfg = normalizeConfig(cfg)
	maxH := maxHorizon(cfg.HorizonDays)
	need := cfg.MinWindow1D + maxH + 1
	if len(daily) < need {
		return Result{}, fmt.Errorf("not enough BTC 1d candles; need at least %d, got %d; run fetch first", need, len(daily))
	}
	params := flow.DefaultParams()
	result := Result{
		GeneratedAt: time.Now(),
		Symbol:      daily[0].Symbol,
		PeriodStart: daily[cfg.MinWindow1D].OpenTime,
		PeriodEnd:   daily[len(daily)-1-maxH].CloseTime,
		Horizons:    append([]int(nil), cfg.HorizonDays...),
		FlowParams:  params,
		FlowCounts:  map[flow.Bias]int{},
		ByBias:      map[flow.Bias]SignalStats{},
	}
	acc := map[flow.Bias]*accStats{}
	for _, b := range allBiases() {
		result.FlowCounts[b] = 0
		acc[b] = newAcc(cfg.HorizonDays)
	}

	for i := cfg.MinWindow1D; i+maxH < len(daily); i++ {
		sig := flow.AnalyzeWithParams(daily[:i+1], "1d", 60, params)
		bias := sig.FlowBias
		if bias == "" {
			bias = flow.BiasNeutral
		}
		if acc[bias] == nil {
			acc[bias] = newAcc(cfg.HorizonDays)
		}
		entry := daily[i].Close
		if entry <= 0 {
			continue
		}
		result.WindowsTested++
		result.FlowCounts[bias]++
		acc[bias].count++
		for _, h := range cfg.HorizonDays {
			future := daily[i+h]
			ret := (future.Close - entry) / entry
			dd := worstDrawdown(daily[i+1:i+h+1], entry)
			acc[bias].returns[h] += ret
			if ret > 0 {
				acc[bias].wins[h]++
			}
			if !acc[bias].initialized[h] || dd < acc[bias].worstDD[h] {
				acc[bias].worstDD[h] = dd
				acc[bias].initialized[h] = true
			}
		}
	}

	for _, b := range allBiases() {
		result.ByBias[b] = finalize(acc[b], cfg.HorizonDays)
	}
	if result.WindowsTested > 0 {
		nonNeutral := result.WindowsTested - result.FlowCounts[flow.BiasNeutral]
		result.SignalDensity = float64(nonNeutral) / float64(result.WindowsTested)
	}
	result.Summary = summarize(result)
	return result, nil
}

func Markdown(r Result) string {
	var b strings.Builder
	b.WriteString("BTC FLOW BACKTEST / AUDIT\n\n")
	b.WriteString("1. Period\n")
	b.WriteString(fmt.Sprintf("- Symbol: %s\n", r.Symbol))
	b.WriteString(fmt.Sprintf("- Windows tested: %d\n", r.WindowsTested))
	b.WriteString(fmt.Sprintf("- Period: %s → %s\n\n", r.PeriodStart.Format("2006-01-02"), r.PeriodEnd.Format("2006-01-02")))

	b.WriteString("2. Detector params\n")
	b.WriteString(fmt.Sprintf("- Volume multiplier: %.2f\n", r.FlowParams.VolumeHighMultiplier))
	b.WriteString(fmt.Sprintf("- Wick ratio: %.2f\n", r.FlowParams.WickRatio))
	b.WriteString(fmt.Sprintf("- Accumulation score: %.2f\n", r.FlowParams.AccumulationScore))
	b.WriteString(fmt.Sprintf("- Distribution score: %.2f\n", r.FlowParams.DistributionScore))
	b.WriteString(fmt.Sprintf("- Trap score: %.2f\n", r.FlowParams.TrapScore))
	b.WriteString(fmt.Sprintf("- Signal density: %.2f%%\n\n", r.SignalDensity*100))

	b.WriteString("3. Flow bias counts\n")
	for _, bias := range allBiases() {
		b.WriteString(fmt.Sprintf("- %s: %d\n", bias, r.FlowCounts[bias]))
	}
	b.WriteString("\n")

	b.WriteString("4. Forward returns\n")
	b.WriteString("| Bias | Count |")
	for _, h := range r.Horizons {
		b.WriteString(fmt.Sprintf(" %dD avg/win |", h))
	}
	b.WriteString("\n|---|---:|")
	for range r.Horizons {
		b.WriteString("---:|")
	}
	b.WriteString("\n")
	for _, bias := range allBiases() {
		stats := r.ByBias[bias]
		b.WriteString(fmt.Sprintf("| %s | %d |", bias, stats.Count))
		for _, h := range r.Horizons {
			b.WriteString(fmt.Sprintf(" %.2f%% / %.1f%% |", stats.AvgReturn[h]*100, stats.WinRate[h]*100))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString("5. Drawdown audit\n")
	b.WriteString("| Bias | Count |")
	for _, h := range r.Horizons {
		b.WriteString(fmt.Sprintf(" %dD worst |", h))
	}
	b.WriteString("\n|---|---:|")
	for range r.Horizons {
		b.WriteString("---:|")
	}
	b.WriteString("\n")
	for _, bias := range allBiases() {
		stats := r.ByBias[bias]
		b.WriteString(fmt.Sprintf("| %s | %d |", bias, stats.Count))
		for _, h := range r.Horizons {
			b.WriteString(fmt.Sprintf(" %.2f%% |", stats.WorstDrawdown[h]*100))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString("6. BTC Flow Detector Bottleneck Audit\n")
	if !r.BTCFlowBottleneckAudit.Enabled {
		b.WriteString("- BTC flow bottleneck audit: skipped / not enough BTC candles\n\n")
	} else {
		b.WriteString("- " + r.BTCFlowBottleneckAudit.Summary + "\n")
		b.WriteString("- Diagnostic only: shows flow component bottlenecks and parameter sensitivity; does not change Flow Engine params.\n")
		b.WriteString("| Component | Count | Rate | Avg bull | Avg bear | Avg conf | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
		limit := len(r.BTCFlowBottleneckAudit.ComponentRows)
		if limit > 16 {
			limit = 16
		}
		for _, row := range r.BTCFlowBottleneckAudit.ComponentRows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %.2f | %.2f | %.2f | %s | %s | %s |\n", row.Component, row.Count, row.Rate*100, row.AvgBullScore, row.AvgBearScore, row.AvgConfidence, btcFlowComponentHorizonCell(row, 3), btcFlowComponentHorizonCell(row, 7), btcFlowComponentHorizonCell(row, 14)))
		}
		b.WriteString("\nBias rows\n")
		b.WriteString("| Bias | Count | Rate | Avg bull | Avg bear | Avg conf | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, row := range r.BTCFlowBottleneckAudit.BiasRows {
			b.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %.2f | %.2f | %.2f | %s | %s | %s |\n", row.Bias, row.Count, row.Rate*100, row.AvgBullScore, row.AvgBearScore, row.AvgConfidence, btcFlowBiasHorizonCell(row, 3), btcFlowBiasHorizonCell(row, 7), btcFlowBiasHorizonCell(row, 14)))
		}
		b.WriteString("\nParameter sensitivity\n")
		b.WriteString("| Params | Signal density | Neutral | Weak score | Bullish | Bearish | Verdict |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")
		for _, row := range r.BTCFlowBottleneckAudit.ParamRows {
			b.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %.1f%% | %s |\n", row.Name, row.SignalDensity*100, row.NeutralRate*100, row.WeakScoreRate*100, row.BullishRate*100, row.BearishRate*100, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("7. Flow Param Candidate Forward Quality Audit\n")
	if !r.FlowParamQualityAudit.Enabled {
		b.WriteString("- Flow param quality audit: skipped / not enough BTC candles\n\n")
	} else {
		b.WriteString("- " + r.FlowParamQualityAudit.Summary + "\n")
		b.WriteString("- Diagnostic only: CANDIDATE_TUNE does not change Flow Engine params automatically.\n")
		b.WriteString("| Params | Bullish | Added bullish | Bearish | 7D bull avg/win/DD | Added 7D avg/win/DD | False+ | Deep DD | Score | Verdict |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
		for _, row := range r.FlowParamQualityAudit.Rows {
			b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s | %s | %.1f%% | %.1f%% | %.4f | %s |\n", row.Name, row.BullishCount, row.AddedBullishCount, row.BearishCount, flowParamQualityBullishHorizonCell(row, 7), flowParamQualityAddedHorizonCell(row, 7), row.FalsePositiveRate*100, row.DeepDrawdownRate*100, row.Score, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("8. BTC Flow by Regime Audit\n")
	if !r.BTCFlowRegimeAudit.Enabled {
		b.WriteString("- BTC flow by regime audit: skipped / not enough BTC candles\n\n")
	} else {
		b.WriteString("- " + r.BTCFlowRegimeAudit.Summary + "\n")
		if note := btcFlowRegimeGuardRecommendation(r.BTCFlowRegimeAudit.Rows); note != "" {
			b.WriteString("- " + note + "\n")
		}
		b.WriteString("- Diagnostic only: compares BTC flow bias inside each market regime; does not change Flow Engine params.\n")
		b.WriteString("| Regime | Bias | Count | Rate | Avg trend | Avg flow | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD | Verdict |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.BTCFlowRegimeAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.BTCFlowRegimeAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %s | %d | %.1f%% | %.1f | %.2f | %s | %s | %s | %s |\n", row.Regime, row.Bias, row.Count, row.Rate*100, row.AvgTrendScore, row.AvgFlowScore, btcFlowRegimeHorizonCell(row, 3), btcFlowRegimeHorizonCell(row, 7), btcFlowRegimeHorizonCell(row, 14), row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("9. BTC Permission Bottleneck Audit\n")
	if !r.BTCPermissionAudit.Enabled {
		b.WriteString("- BTC permission audit: skipped / not enough BTC candles\n\n")
	} else {
		b.WriteString("- " + r.BTCPermissionAudit.Summary + "\n")
		b.WriteString("- Diagnostic only: explains why Agent 1 does or does not reach ALLOWED; does not tune rules, create alerts, or place orders.\n")
		b.WriteString("| Permission | Count | Rate | Avg trend | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
		for _, row := range r.BTCPermissionAudit.Rows {
			b.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %.1f | %s | %s | %s |\n", row.Permission, row.Count, row.Rate*100, row.AvgTrendScore, btcPermissionHorizonCell(row, 3), btcPermissionHorizonCell(row, 7), btcPermissionHorizonCell(row, 14)))
		}
		if len(r.BTCPermissionAudit.Blockers) > 0 {
			b.WriteString("\nTop blockers\n")
			b.WriteString("| Blocker | Count | Rate |\n")
			b.WriteString("|---|---:|---:|\n")
			limit := len(r.BTCPermissionAudit.Blockers)
			if limit > 12 {
				limit = 12
			}
			for _, row := range r.BTCPermissionAudit.Blockers[:limit] {
				b.WriteString(fmt.Sprintf("| %s | %d | %.1f%% |\n", row.Blocker, row.Count, row.Rate*100))
			}
		}
		if len(r.BTCPermissionAudit.BlockersByPermission) > 0 {
			b.WriteString("\nTop blockers by permission\n")
			b.WriteString("| Permission | Blocker | Count | Rate within permission |\n")
			b.WriteString("|---|---|---:|---:|\n")
			shown := map[agent1.Permission]int{}
			for _, row := range r.BTCPermissionAudit.BlockersByPermission {
				if shown[row.Permission] >= 5 {
					continue
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %d | %.1f%% |\n", row.Permission, row.Blocker, row.Count, row.RateWithinPermission*100))
				shown[row.Permission]++
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("10. Agent 2 Layer Simulation\n")
	if !r.Agent2Simulation.Enabled {
		b.WriteString("- Agent 2 simulation: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.Agent2Simulation.Summary + "\n")
		for _, sym := range sortedAssetSymbols(r.Agent2Simulation.Assets) {
			asset := r.Agent2Simulation.Assets[sym]
			b.WriteString(fmt.Sprintf("\n%s\n", sym))
			b.WriteString(fmt.Sprintf("- plans created: %d\n", asset.PlansCreated))
			b.WriteString(fmt.Sprintf("- orders placed/filled/expired: %d / %d / %d\n", asset.OrdersPlaced, asset.OrdersFilled, asset.OrdersExpired))
			b.WriteString(fmt.Sprintf("- fill rate: %.2f%%\n", asset.FillRate*100))
			b.WriteString(fmt.Sprintf("- invalidations / take-profits / time-stops: %d / %d / %d\n", asset.Invalidations, asset.TakeProfits, asset.TimeStops))
			b.WriteString(fmt.Sprintf("- max deployed: %.2f\n", asset.MaxDeployed))
			b.WriteString(fmt.Sprintf("- max drawdown: %.2f%%\n", asset.MaxDrawdown*100))
			b.WriteString(fmt.Sprintf("- final simulated PnL: %.2f\n", asset.FinalPnL))
		}
		b.WriteString("\nDiagnostics\n")
		d := r.Agent2Simulation.Diagnostics
		b.WriteString(fmt.Sprintf("- windows tested: %d\n", d.WindowsTested))
		b.WriteString(fmt.Sprintf("- Agent 1 permissions: %s\n", permissionCounts(d.Agent1PermissionCount)))
		b.WriteString(fmt.Sprintf("- Agent 1 regimes: %s\n", stringCounts(d.Agent1RegimeCounts, 6)))
		b.WriteString(fmt.Sprintf("- Agent 1 risks: %s\n", stringCounts(d.Agent1RiskCounts, 6)))
		b.WriteString("- Top asset block reasons:\n")
		for _, sym := range sortedReasonSymbols(d.AssetReasonCounts) {
			b.WriteString(fmt.Sprintf("  - %s: %s\n", sym, topReasons(d.AssetReasonCounts[sym], 5)))
		}
		if len(d.Events) > 0 {
			b.WriteString("- Event samples:\n")
			limit := len(d.Events)
			if limit > 12 {
				limit = 12
			}
			for _, event := range d.Events[:limit] {
				b.WriteString(fmt.Sprintf("  - %s %s %s layer=%d price=%.4f invalidation=%.4f %s\n", event.Time, event.Symbol, event.Type, event.Layer, event.Price, event.Invalidation, event.Reason))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("11. Agent 2 ARMED Research Simulation\n")
	b.WriteString("- Research-only: treats ARMED as ALLOWED inside this backtest simulation only; production plan/live behavior unchanged.\n")
	if !r.Agent2ArmedResearchSimulation.Enabled {
		b.WriteString("- Agent 2 ARMED research simulation: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.Agent2ArmedResearchSimulation.Summary + "\n")
		for _, sym := range sortedAssetSymbols(r.Agent2ArmedResearchSimulation.Assets) {
			asset := r.Agent2ArmedResearchSimulation.Assets[sym]
			b.WriteString(fmt.Sprintf("\n%s\n", sym))
			b.WriteString(fmt.Sprintf("- plans created: %d\n", asset.PlansCreated))
			b.WriteString(fmt.Sprintf("- orders placed/filled/expired: %d / %d / %d\n", asset.OrdersPlaced, asset.OrdersFilled, asset.OrdersExpired))
			b.WriteString(fmt.Sprintf("- fill rate: %.2f%%\n", asset.FillRate*100))
			b.WriteString(fmt.Sprintf("- invalidations / take-profits / time-stops: %d / %d / %d\n", asset.Invalidations, asset.TakeProfits, asset.TimeStops))
			b.WriteString(fmt.Sprintf("- max deployed: %.2f\n", asset.MaxDeployed))
			b.WriteString(fmt.Sprintf("- max drawdown: %.2f%%\n", asset.MaxDrawdown*100))
			b.WriteString(fmt.Sprintf("- final simulated PnL: %.2f\n", asset.FinalPnL))
		}
		b.WriteString("\nDiagnostics\n")
		d := r.Agent2ArmedResearchSimulation.Diagnostics
		b.WriteString(fmt.Sprintf("- windows tested: %d\n", d.WindowsTested))
		b.WriteString(fmt.Sprintf("- Agent 1 permissions: %s\n", permissionCounts(d.Agent1PermissionCount)))
		b.WriteString(fmt.Sprintf("- Agent 1 regimes: %s\n", stringCounts(d.Agent1RegimeCounts, 6)))
		b.WriteString(fmt.Sprintf("- Agent 1 risks: %s\n", stringCounts(d.Agent1RiskCounts, 6)))
		b.WriteString("- Top asset block reasons:\n")
		for _, sym := range sortedReasonSymbols(d.AssetReasonCounts) {
			b.WriteString(fmt.Sprintf("  - %s: %s\n", sym, topReasons(d.AssetReasonCounts[sym], 5)))
		}
		if len(d.Events) > 0 {
			b.WriteString("- Event samples:\n")
			limit := len(d.Events)
			if limit > 12 {
				limit = 12
			}
			for _, event := range d.Events[:limit] {
				b.WriteString(fmt.Sprintf("  - %s %s %s layer=%d price=%.4f invalidation=%.4f %s\n", event.Time, event.Symbol, event.Type, event.Layer, event.Price, event.Invalidation, event.Reason))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("12. Agent 2 Watchlist Trigger Audit\n")
	if !r.WatchlistTriggerAudit.Enabled {
		b.WriteString("- Watchlist trigger audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.WatchlistTriggerAudit.Summary + "\n")
		b.WriteString("- Tuned audit focuses on actionable watch candidates by default; noisy BTC-not-allowed/relative-weak rows are skipped unless explicitly included.\n")
		b.WriteString("| Symbol | Trigger | Ready>= | Count | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD | Score | Verdict |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.WatchlistTriggerAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.WatchlistTriggerAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %s | %.2f | %d | %s | %s | %s | %.2f | %s |\n", row.Symbol, row.Trigger, row.ReadinessThreshold, row.Count, watchAuditHorizonCell(row, 3), watchAuditHorizonCell(row, 7), watchAuditHorizonCell(row, 14), row.Score, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("13. Agent 2 Near-Miss Watchlist Forward Audit\n")
	if !r.NearMissWatchlistAudit.Enabled {
		b.WriteString("- Near-miss watchlist audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.NearMissWatchlistAudit.Summary + "\n")
		b.WriteString("- Research-only: includes unactionable/noisy candidates for diagnosis; does not create alerts or orders.\n")
		b.WriteString("| Symbol | Trigger | Ready>= | Count | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD | Score | Verdict |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.NearMissWatchlistAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.NearMissWatchlistAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %s | %.2f | %d | %s | %s | %s | %.2f | %s |\n", row.Symbol, row.Trigger, row.ReadinessThreshold, row.Count, watchAuditHorizonCell(row, 3), watchAuditHorizonCell(row, 7), watchAuditHorizonCell(row, 14), row.Score, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("14. Agent 2 Asset Flow Entry Forward Audit\n")
	if !r.AssetFlowEntryAudit.Enabled {
		b.WriteString("- Asset flow entry audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.AssetFlowEntryAudit.Summary + "\n")
		b.WriteString("- Diagnostic only: measures AssetFlowEntry pass/soft-fail/hard-block forward quality; does not change thresholds, alerts, plans, or orders.\n")
		b.WriteString("| Symbol | Flow bias | Trigger | Bull bucket | Count | Avg bull | Avg bear | 3D avg/win/DD | 7D avg/win/DD | 14D avg/win/DD | Score | Verdict |\n")
		b.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.AssetFlowEntryAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.AssetFlowEntryAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %.2f | %.2f | %s | %s | %s | %.2f | %s |\n", row.Symbol, row.FlowBias, row.Trigger, row.BullScoreBucket, row.Count, row.AvgBullScore, row.AvgBearScore, assetFlowEntryAuditHorizonCell(row, 3), assetFlowEntryAuditHorizonCell(row, 7), assetFlowEntryAuditHorizonCell(row, 14), row.Score, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("15. Agent 2 Near-Miss Forced Layer Mechanics Audit\n")
	b.WriteString("- Research-only forced near-miss layer audit; production plan/live behavior unchanged.\n")
	if !r.NearMissLayerAudit.Enabled {
		b.WriteString("- Near-miss forced layer audit: skipped / not enough candidate candles\n\n")
	} else {
		b.WriteString("- " + r.NearMissLayerAudit.Summary + "\n")
		b.WriteString("| Symbol | Trigger | Ready>= | Inv buffer | TP | Time stop | Plans | Filled | Expired | TP hits | Invalidations | Time stops | Max DD | PnL | Score | Verdict |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.NearMissLayerAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.NearMissLayerAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %s | %.2f | %.3f | %.2f%% | %d | %d | %d | %d | %d | %d | %d | %.2f%% | %.2f | %.2f | %s |\n", row.Symbol, row.Trigger, row.ReadinessThreshold, row.InvalidationBuffer, row.TakeProfitPct*100, row.TimeStopDays, row.PlansCreated, row.OrdersFilled, row.OrdersExpired, row.TakeProfits, row.Invalidations, row.TimeStops, row.MaxDrawdown*100, row.FinalPnL, row.Score, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("16. Agent 2 Checklist Pass-Count Audit\n")
	if !r.ChecklistPassCountAudit.Enabled {
		b.WriteString("- Checklist pass-count audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.ChecklistPassCountAudit.Summary + "\n")
		b.WriteString("- Diagnostic only: counts deterministic checklist pass/fail blockers; does not loosen rules or create alerts/orders.\n")
		b.WriteString("| Symbol | Samples | Avg pass | Hard fail % | Soft fail % | Near-actionable | Top hard blocker | Top soft wait | Verdict |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---|---|---|\n")
		limit := len(r.ChecklistPassCountAudit.Rows)
		if limit > 18 {
			limit = 18
		}
		for _, row := range r.ChecklistPassCountAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %d | %.1f / %.1f | %.1f%% | %.1f%% | %d | %s | %s | %s |\n", row.Symbol, row.Samples, row.AvgPassedChecks, row.AvgTotalChecks, row.HardFailRate*100, row.SoftFailRate*100, row.NearActionableCount, emptyDash(row.TopHardBlocker), emptyDash(row.TopSoftWait), row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("17. Agent 2 Invalidation/Layer Audit\n")
	if !r.LayerAudit.Enabled {
		b.WriteString("- Layer audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.LayerAudit.Summary + "\n")
		b.WriteString("| Symbol | Inv buffer | Layer depth | Plans | Filled | Expired | Invalidations | Max DD | PnL | Verdict |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.LayerAudit.Rows)
		if limit > 18 {
			limit = 18
		}
		for _, row := range r.LayerAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %.3f | %.2f | %d | %d | %d | %d | %.2f%% | %.2f | %s |\n", row.Symbol, row.InvalidationBuffer, row.LayerDepthMultiplier, row.PlansCreated, row.OrdersFilled, row.OrdersExpired, row.Invalidations, row.MaxDrawdown*100, row.FinalPnL, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("18. Agent 2 Exit / Take-Profit Audit\n")
	if !r.ExitAudit.Enabled {
		b.WriteString("- Exit audit: skipped / not enough asset candles\n\n")
	} else {
		b.WriteString("- " + r.ExitAudit.Summary + "\n")
		b.WriteString("| Symbol | TP | Time stop | Plans | Filled | TP hits | Time stops | Invalidations | Max DD | PnL | Verdict |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
		limit := len(r.ExitAudit.Rows)
		if limit > 24 {
			limit = 24
		}
		for _, row := range r.ExitAudit.Rows[:limit] {
			b.WriteString(fmt.Sprintf("| %s | %.2f%% | %d | %d | %d | %d | %d | %d | %.2f%% | %.2f | %s |\n", row.Symbol, row.TakeProfitPct*100, row.TimeStopDays, row.PlansCreated, row.OrdersFilled, row.TakeProfits, row.TimeStops, row.Invalidations, row.MaxDrawdown*100, row.FinalPnL, row.Verdict))
		}
		b.WriteString("\n")
	}

	b.WriteString("19. Kết luận\n")
	b.WriteString("- " + r.Summary + "\n")
	b.WriteString("- Đây là audit rule bằng dữ liệu quá khứ, không phải cam kết lợi nhuận. Mẫu ít thì chỉ dùng để debug rule. Agent 2 simulation chưa mô hình take-profit.\n")
	return b.String()
}

func flowParamQualityBullishHorizonCell(row FlowParamQualityAuditRow, horizon int) string {
	if row.BullishAvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.BullishAvgReturn[horizon]*100, row.BullishWinRate[horizon]*100, row.BullishWorstDrawdown[horizon]*100)
}

func flowParamQualityAddedHorizonCell(row FlowParamQualityAuditRow, horizon int) string {
	if row.AddedAvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AddedAvgReturn[horizon]*100, row.AddedWinRate[horizon]*100, row.AddedWorstDrawdown[horizon]*100)
}

func btcFlowComponentHorizonCell(row BTCFlowComponentAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func btcFlowBiasHorizonCell(row BTCFlowBiasAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func btcFlowRegimeHorizonCell(row BTCFlowRegimeAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func btcPermissionHorizonCell(row BTCPermissionAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func assetFlowEntryAuditHorizonCell(row AssetFlowEntryAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func watchAuditHorizonCell(row WatchlistTriggerAuditRow, horizon int) string {
	if row.AvgReturn == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f%% / %.1f%% / %.2f%%", row.AvgReturn[horizon]*100, row.WinRate[horizon]*100, row.WorstDrawdown[horizon]*100)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func SaveReports(dir string, r Result, markdown string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "backtest_latest.md"), []byte(markdown), 0600); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "backtest_latest.json"), b, 0600)
}

func normalizeConfig(cfg Config) Config {
	if cfg.MinWindow1D <= 0 {
		cfg.MinWindow1D = 60
	}
	if len(cfg.HorizonDays) == 0 {
		cfg.HorizonDays = []int{1, 3, 7, 14}
	}
	out := cfg.HorizonDays[:0]
	seen := map[int]bool{}
	for _, h := range cfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	sort.Ints(out)
	cfg.HorizonDays = out
	return cfg
}

func newAcc(horizons []int) *accStats {
	a := &accStats{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func finalize(a *accStats, horizons []int) SignalStats {
	stats := SignalStats{AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	if a == nil {
		return stats
	}
	stats.Count = a.count
	for _, h := range horizons {
		if a.count > 0 {
			stats.AvgReturn[h] = a.returns[h] / float64(a.count)
			stats.WinRate[h] = float64(a.wins[h]) / float64(a.count)
			stats.WorstDrawdown[h] = a.worstDD[h]
		}
	}
	return stats
}

func worstDrawdown(c []market.Candle, entry float64) float64 {
	if len(c) == 0 || entry <= 0 {
		return 0
	}
	worst := 0.0
	for _, candle := range c {
		dd := (candle.Low - entry) / entry
		if dd < worst {
			worst = dd
		}
	}
	return worst
}

func summarize(r Result) string {
	if r.WindowsTested == 0 {
		return "Không có window hợp lệ để audit."
	}
	bestBias := flow.BiasNeutral
	bestReturn := -999.0
	lastH := r.Horizons[len(r.Horizons)-1]
	for _, bias := range allBiases() {
		stats := r.ByBias[bias]
		if stats.Count < 5 {
			continue
		}
		if stats.AvgReturn[lastH] > bestReturn {
			bestReturn = stats.AvgReturn[lastH]
			bestBias = bias
		}
	}
	densityNote := "mật độ tín hiệu nằm trong vùng audit hợp lý"
	if r.SignalDensity < 0.03 {
		densityNote = "mật độ tín hiệu còn quá thấp; detector vẫn bảo thủ, mẫu ít"
	} else if r.SignalDensity > 0.25 {
		densityNote = "mật độ tín hiệu cao; có nguy cơ nhiễu/spam signal"
	}
	if bestReturn == -999.0 {
		return fmt.Sprintf("%s; chỉ dùng để debug rule, chưa đủ kết luận thống kê.", densityNote)
	}
	return fmt.Sprintf("%s. Trong mẫu hiện có, bias %s có forward return %dD trung bình tốt nhất %.2f%%; vẫn cần kiểm tra thêm vì backtest chỉ dùng OHLCV.", densityNote, bestBias, lastH, bestReturn*100)
}

func maxHorizon(horizons []int) int {
	max := 0
	for _, h := range horizons {
		if h > max {
			max = h
		}
	}
	return max
}

func allBiases() []flow.Bias {
	return []flow.Bias{flow.BiasNeutral, flow.BiasAccumulation, flow.BiasBearTrap, flow.BiasBullTrap, flow.BiasDistribution}
}

func sortedAssetSymbols(assets map[string]AssetSimStats) []string {
	out := make([]string, 0, len(assets))
	for sym := range assets {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

func sortedReasonSymbols(reasons map[string]map[string]int) []string {
	out := make([]string, 0, len(reasons))
	for sym := range reasons {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

func permissionCounts(counts map[agent1.Permission]int) string {
	if len(counts) == 0 {
		return "none"
	}
	order := []agent1.Permission{agent1.Allowed, agent1.Armed, agent1.Watch, agent1.NoTrade}
	parts := make([]string, 0, len(order))
	for _, perm := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", perm, counts[perm]))
	}
	return strings.Join(parts, "; ")
}

func stringCounts(counts map[string]int, limit int) string {
	if len(counts) == 0 {
		return "none"
	}
	type pair struct {
		key string
		val int
	}
	pairs := make([]pair, 0, len(counts))
	for key, val := range counts {
		pairs = append(pairs, pair{key: key, val: val})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].val == pairs[j].val {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].val > pairs[j].val
	})
	if limit > 0 && len(pairs) > limit {
		pairs = pairs[:limit]
	}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s=%d", p.key, p.val)
	}
	return strings.Join(parts, "; ")
}
