package main

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/backtest"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/learning"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
	"btc-agent/internal/survey"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func buildBacktestResult(cfg config.Config, db *storage.DB) (backtest.Result, error) {
	daily, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return backtest.Result{}, err
	}
	result, err := backtest.RunBTC(backtest.Config{MinWindow1D: 60, HorizonDays: []int{1, 3, 7, 14}}, daily)
	if err != nil {
		return backtest.Result{}, err
	}
	btc, err := loadBTC(cfg, db)
	if err != nil {
		btc = map[string][]market.Candle{"1d": daily}
	}
	btc["1d"] = daily
	flowAudit, err := backtest.RunBTCFlowBottleneckAudit(btc, backtest.BTCFlowBottleneckAuditConfig{})
	if err != nil {
		result.BTCFlowBottleneckAudit = backtest.BTCFlowBottleneckAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.BTCFlowBottleneckAudit = flowAudit
	}
	qualityAudit, err := backtest.RunFlowParamQualityAudit(btc, backtest.FlowParamQualityAuditConfig{})
	if err != nil {
		result.FlowParamQualityAudit = backtest.FlowParamQualityAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.FlowParamQualityAudit = qualityAudit
	}
	flowRegimeAudit, err := backtest.RunBTCFlowRegimeAudit(cfg, btc, backtest.BTCFlowRegimeAuditConfig{})
	if err != nil {
		result.BTCFlowRegimeAudit = backtest.BTCFlowRegimeAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.BTCFlowRegimeAudit = flowRegimeAudit
	}
	permissionAudit, err := backtest.RunBTCPermissionAudit(cfg, btc, backtest.BTCPermissionAuditConfig{})
	if err != nil {
		result.BTCPermissionAudit = backtest.BTCPermissionAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.BTCPermissionAudit = permissionAudit
	}
	thresholdCalibration, err := backtest.RunThresholdCalibration(cfg, btc, backtest.ThresholdCalibrationConfig{})
	if err != nil {
		result.ThresholdCalibration = backtest.ThresholdCalibrationResult{Enabled: false, Summary: err.Error()}
	} else {
		result.ThresholdCalibration = thresholdCalibration
	}
	assets := map[string][]market.Candle{}
	for _, sym := range cfg.Data.Symbols.Assets {
		candles, err := db.LoadCandles(sym, "1d", cfg.Data.CandleLimit)
		if err != nil {
			continue
		}
		assets[sym] = candles
	}
	if analysis, err := agent1.Analyze(cfg, btc, exchange.FearGreed{Value: 50, Classification: "Neutral"}); err == nil {
		result.DataSanity = liveguard.CheckDataSanity(cfg, btc, assets, analysis, time.Now())
	}
	result.ZoneEntrySanity = backtest.RunZoneEntrySanity(cfg, assets)
	result.MMAccumulationAudit = backtest.RunMMAccumulationAudit(cfg, assets)
	result.AccumulationPhaseAudit = backtest.RunAccumulationPhaseAudit(cfg.Data.Symbols.BTC, daily, []int{1, 3, 7, 14, 30})
	sim, err := backtest.RunAgent2Simulation(cfg, btc, assets)
	if err != nil {
		result.Agent2Simulation = backtest.Agent2Simulation{Enabled: false, Assets: map[string]backtest.AssetSimStats{}, Summary: err.Error()}
	} else {
		result.Agent2Simulation = sim
	}
	armedResearchSim, err := backtest.RunAgent2SimulationWithOverrides(cfg, btc, assets, backtest.SimulationOverrides{AllowArmedAsAllowed: true})
	if err != nil {
		result.Agent2ArmedResearchSimulation = backtest.Agent2Simulation{Enabled: false, Assets: map[string]backtest.AssetSimStats{}, Summary: err.Error()}
	} else {
		result.Agent2ArmedResearchSimulation = armedResearchSim
	}

	walkForward, err := backtest.RunWalkForwardSimulation(cfg, btc, assets, 3, 0.6)
	if err == nil {
		result.WalkForwardReport = walkForward
		if mc, mcErr := backtest.RunMonteCarloRobustness(walkForward, 5000, 42); mcErr == nil {
			result.MonteCarloReport = mc
		}
	}
	watchAudit, err := backtest.RunWatchlistTriggerAudit(cfg, btc, assets, backtest.WatchlistTriggerAuditConfig{})
	if err != nil {
		result.WatchlistTriggerAudit = backtest.WatchlistTriggerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.WatchlistTriggerAudit = watchAudit
	}
	nearMissAudit, err := backtest.RunWatchlistTriggerAudit(cfg, btc, assets, backtest.WatchlistTriggerAuditConfig{IncludeUnactionable: true, ReadinessThresholds: []float64{0.35, 0.45, 0.55, 0.60}, HorizonDays: []int{3, 7, 14}})
	if err != nil {
		result.NearMissWatchlistAudit = backtest.WatchlistTriggerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.NearMissWatchlistAudit = nearMissAudit
	}
	assetFlowAudit, err := backtest.RunAssetFlowEntryAudit(cfg, assets, backtest.AssetFlowEntryAuditConfig{})
	if err != nil {
		result.AssetFlowEntryAudit = backtest.AssetFlowEntryAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.AssetFlowEntryAudit = assetFlowAudit
	}
	nearMissLayerAudit, err := backtest.RunNearMissLayerAudit(cfg, btc, assets, backtest.NearMissLayerAuditConfig{})
	if err != nil {
		result.NearMissLayerAudit = backtest.NearMissLayerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.NearMissLayerAudit = nearMissLayerAudit
	}
	checklistAudit, err := backtest.RunChecklistPassCountAudit(cfg, btc, assets, backtest.ChecklistPassCountAuditConfig{})
	if err != nil {
		result.ChecklistPassCountAudit = backtest.ChecklistPassCountAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.ChecklistPassCountAudit = checklistAudit
	}
	opportunityAudit, err := backtest.RunAgent2OpportunityAudit(cfg, btc, assets, backtest.Agent2OpportunityAuditConfig{})
	if err != nil {
		result.Agent2OpportunityAudit = backtest.Agent2OpportunityAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.Agent2OpportunityAudit = opportunityAudit
	}
	filterValueAudit, err := backtest.RunFilterValueAudit(cfg, btc, assets, backtest.FilterValueAuditConfig{})
	if err != nil {
		result.FilterValueAudit = backtest.FilterValueAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.FilterValueAudit = filterValueAudit
	}
	audit, err := backtest.RunLayerAudit(cfg, btc, assets, backtest.LayerAuditConfig{})
	if err != nil {
		result.LayerAudit = backtest.LayerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.LayerAudit = audit
	}
	exitAudit, err := backtest.RunExitAudit(cfg, btc, assets, backtest.ExitAuditConfig{})
	if err != nil {
		result.ExitAudit = backtest.ExitAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.ExitAudit = exitAudit
	}
	return result, nil
}

func runBacktest(cfg config.Config, db *storage.DB) error {
	result, err := buildBacktestResult(cfg, db)
	if err != nil {
		return err
	}
	md := backtest.Markdown(result)
	if err := backtest.SaveReports("reports", result, md); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func runBacktestLiveManager(cfg config.Config, db *storage.DB, researchArmed bool, researchProfile string, researchExpiryDays int, researchHoldWatch bool, researchHoldPriceAboveDiscountPct float64, productionArmedProbe bool) error {
	btc, err := loadBTC(cfg, db)
	if err != nil {
		return err
	}
	assets := map[string][]market.Candle{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		candles, err := db.LoadCandles(symbol, "1d", cfg.Data.CandleLimit)
		if err != nil {
			continue
		}
		assets[symbol] = candles
	}
	result, err := liveguard.RunLiveManagerHistorySimulationWithOptions(cfg, btc, assets, liveguard.LiveManagerHistoryOptions{ResearchArmed: researchArmed, ResearchProfile: researchProfile, ResearchExpiryDays: researchExpiryDays, ResearchHoldWatch: researchHoldWatch, ResearchHoldPriceAboveDiscountPct: researchHoldPriceAboveDiscountPct, ProductionArmedProbe: productionArmedProbe})
	if err != nil {
		return err
	}
	if err := saveJSONFile("reports", "live_manager_history_latest.json", result); err != nil {
		return err
	}
	md := liveManagerHistoryMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_manager_history_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func liveManagerHistoryMarkdown(result liveguard.LiveManagerHistoryResult) string {
	md := "LIVE MANAGER HISTORICAL SIMULATION\n\n"
	md += fmt.Sprintf("Summary: %s\n", result.Summary)
	if result.ProductionArmedProbe {
		md += "Mode: PRODUCTION_ARMED_PROBE — WATCH no order, ARMED one probe, ALLOWED normal ladder. Historical only.\n"
	}
	if result.ResearchArmed || result.ResearchProfile != "" || result.ResearchExpiryDays > 0 || result.ResearchHoldWatch || result.ResearchHoldPriceAboveDiscountPct > 0 {
		md += "Mode: RESEARCH"
		if result.ResearchArmed {
			md += " | ARMED->ALLOWED"
		}
		if result.ResearchProfile != "" {
			md += " | profile=" + result.ResearchProfile
		}
		if result.ResearchExpiryDays > 0 {
			md += fmt.Sprintf(" | expiry=%dd", result.ResearchExpiryDays)
		}
		if result.ResearchHoldWatch {
			md += " | hold-through-watch"
		}
		if result.ResearchHoldPriceAboveDiscountPct > 0 {
			md += fmt.Sprintf(" | hold-above-discount=%.2f%%", result.ResearchHoldPriceAboveDiscountPct*100)
		}
		md += " — backtest only. Production/live unchanged.\n"
	}
	md += fmt.Sprintf("Period: %s -> %s\n", result.PeriodStart.Format("2006-01-02"), result.PeriodEnd.Format("2006-01-02"))
	md += fmt.Sprintf("Windows tested: %d\n\n", result.WindowsTested)
	md += "Totals:\n"
	md += fmt.Sprintf("- Desired: %d\n", result.Total.Desired)
	md += fmt.Sprintf("- Placed: %d\n", result.Total.Placed)
	md += fmt.Sprintf("- Filled: %d\n", result.Total.Filled)
	md += fmt.Sprintf("- Canceled: %d\n", result.Total.Canceled)
	md += fmt.Sprintf("- Replaced: %d\n", result.Total.Replaced)
	md += fmt.Sprintf("- Blocked: %d\n", result.Total.Blocked)
	md += fmt.Sprintf("- Expired: %d\n", result.Total.Expired)
	md += fmt.Sprintf("- Fill rate: %.2f%%\n", result.Total.FillRate*100)
	md += fmt.Sprintf("- Cancel rate: %.2f%%\n", result.Total.CancelRate*100)
	md += fmt.Sprintf("- Replace rate: %.2f%%\n\n", result.Total.ReplaceRate*100)
	if len(result.Total.Blockers) > 0 {
		md += "Top blockers:\n"
		for _, blocker := range firstHistoryBlockers(result.Total.Blockers, 12) {
			md += fmt.Sprintf("- %s: %d\n", blocker, result.Total.Blockers[blocker])
		}
		md += "\n"
	}
	md += fmt.Sprintf("Quality: %s %.1f/100 — %s\n\n", result.Total.QualityGrade, result.Total.QualityScore, result.Total.QualityReason)
	if result.ProductionArmedProbe {
		md += "ARMED probe stats:\n"
		md += fmt.Sprintf("- Desired/Placed/Filled/Canceled/Replaced/Blocked: %d / %d / %d / %d / %d / %d\n", result.ArmedProbe.Desired, result.ArmedProbe.Placed, result.ArmedProbe.Filled, result.ArmedProbe.Canceled, result.ArmedProbe.Replaced, result.ArmedProbe.Blocked)
		md += fmt.Sprintf("- Fill rate: %.2f%% | Cancel rate: %.2f%% | Quality: %s %.1f/100\n", result.ArmedProbe.FillRate*100, result.ArmedProbe.CancelRate*100, result.ArmedProbe.QualityGrade, result.ArmedProbe.QualityScore)
		if len(result.ArmedProbe.Blockers) > 0 {
			md += "- Top ARMED blockers: "
			parts := []string{}
			for _, blocker := range firstHistoryBlockers(result.ArmedProbe.Blockers, 5) {
				parts = append(parts, fmt.Sprintf("%s=%d", blocker, result.ArmedProbe.Blockers[blocker]))
			}
			md += strings.Join(parts, "; ") + "\n"
		}
		md += "\n"
	}
	if len(result.Total.CancelReasons) > 0 {
		md += "Cancel reasons:\n"
		for _, reason := range firstHistoryCancelReasons(result.Total.CancelReasons, 12) {
			md += fmt.Sprintf("- %s: %d\n", reason, result.Total.CancelReasons[reason])
		}
		md += "\n"
	}
	if len(result.Total.DesiredLoss) > 0 {
		md += "Desired-loss reasons:\n"
		for _, reason := range firstHistoryDesiredLoss(result.Total.DesiredLoss, 12) {
			md += fmt.Sprintf("- %s: %d\n", reason, result.Total.DesiredLoss[reason])
		}
		md += "\n"
	}
	md += "Per coin:\n"
	for _, symbol := range liveguard.SortedHistorySymbols(result.PerCoin) {
		stats := result.PerCoin[symbol]
		md += fmt.Sprintf("\n%s\n", symbol)
		md += fmt.Sprintf("- Desired: %d\n", stats.Desired)
		md += fmt.Sprintf("- Placed/Filled/Canceled/Replaced/Blocked/Expired: %d / %d / %d / %d / %d / %d\n", stats.Placed, stats.Filled, stats.Canceled, stats.Replaced, stats.Blocked, stats.Expired)
		md += fmt.Sprintf("- Fill rate: %.2f%%\n", stats.FillRate*100)
		md += fmt.Sprintf("- Cancel rate: %.2f%%\n", stats.CancelRate*100)
		if stats.QualityGrade != "" {
			md += fmt.Sprintf("- Quality: %s %.1f/100 — %s\n", stats.QualityGrade, stats.QualityScore, stats.QualityReason)
		}
		if stats.BestLayer > 0 {
			md += fmt.Sprintf("- Best layer: %d\n", stats.BestLayer)
		} else {
			md += "- Best layer: n/a\n"
		}
		if len(stats.CancelReasons) > 0 {
			md += "- Cancel reasons: "
			parts := []string{}
			for _, reason := range firstHistoryCancelReasons(stats.CancelReasons, 5) {
				parts = append(parts, fmt.Sprintf("%s=%d", reason, stats.CancelReasons[reason]))
			}
			md += strings.Join(parts, "; ") + "\n"
		}
		if len(stats.DesiredLoss) > 0 {
			md += "- Desired-loss reasons: "
			parts := []string{}
			for _, reason := range firstHistoryDesiredLoss(stats.DesiredLoss, 5) {
				parts = append(parts, fmt.Sprintf("%s=%d", reason, stats.DesiredLoss[reason]))
			}
			md += strings.Join(parts, "; ") + "\n"
		}
		if len(stats.Blockers) > 0 {
			md += "- Top blockers: "
			parts := []string{}
			for _, blocker := range firstHistoryBlockers(stats.Blockers, 5) {
				parts = append(parts, fmt.Sprintf("%s=%d", blocker, stats.Blockers[blocker]))
			}
			md += strings.Join(parts, "; ") + "\n"
		}
	}
	if len(result.Events) > 0 {
		if diagnostics := historyCancelDiagnosticLines(result.Events, 20); len(diagnostics) > 0 {
			md += "\nCancel diagnostics:\n"
			for _, line := range diagnostics {
				md += "- " + line + "\n"
			}
		}
		md += "\nEvents:\n"
		limit := len(result.Events)
		if limit > 40 {
			limit = 40
		}
		for _, event := range result.Events[:limit] {
			md += fmt.Sprintf("- %s %s %s layer=%d price=%.8f notional=%.2f %s\n", event.Time, event.Symbol, event.Type, event.Layer, event.Price, event.Notional, event.Reason)
		}
	}
	if len(result.Notes) > 0 {
		md += "\nNotes:\n"
		for _, note := range result.Notes {
			md += "- " + note + "\n"
		}
	}
	md += "\nNo real order was placed or canceled. Historical simulation only.\n"
	return md
}

func historyCancelDiagnosticLines(events []liveguard.LiveManagerHistoryEvent, limit int) []string {
	out := []string{}
	for _, event := range events {
		if event.Type != "CANCEL" || !strings.Contains(event.Reason, "diag close=") {
			continue
		}
		out = append(out, fmt.Sprintf("%s %s layer=%d price=%.8f %s", event.Time, event.Symbol, event.Layer, event.Price, event.Reason))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func firstHistoryBlockers(blockers map[string]int, limit int) []string {
	items := liveguard.SortedHistoryBlockers(blockers)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func firstHistoryCancelReasons(reasons map[string]int, limit int) []string {
	items := liveguard.SortedHistoryCancelReasons(reasons)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func firstHistoryDesiredLoss(reasons map[string]int, limit int) []string {
	items := liveguard.SortedHistoryDesiredLoss(reasons)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func runLearning(cfg config.Config, db *storage.DB) error {
	backtestResult, err := buildBacktestResult(cfg, db)
	if err != nil {
		return err
	}
	history, historyOK := loadLiveManagerHistoryReport()
	var historyPtr *liveguard.LiveManagerHistoryResult
	if historyOK {
		historyPtr = &history
	}
	surveyResult := survey.BuildWithMicrostructure(backtestResult, historyPtr, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
	surveyMarkdown := survey.Markdown(surveyResult)
	if err := survey.SaveReports("reports", surveyResult, surveyMarkdown); err != nil {
		return err
	}
	result := learning.BuildRecommendationsWithSurvey(backtestResult, surveyResult)
	md := learning.Markdown(result)
	if err := learning.SaveReports("reports", result, md); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func runRealDataSurvey(cfg config.Config, db *storage.DB) error {
	backtestResult, err := buildBacktestResult(cfg, db)
	if err != nil {
		return err
	}
	history, historyOK := loadLiveManagerHistoryReport()
	var historyPtr *liveguard.LiveManagerHistoryResult
	if historyOK {
		historyPtr = &history
	}
	result := survey.BuildWithMicrostructure(backtestResult, historyPtr, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
	md := survey.Markdown(result)
	if err := survey.SaveReports("reports", result, md); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func loadLiveManagerHistoryReport() (liveguard.LiveManagerHistoryResult, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "live_manager_history_latest.json"))
	if err != nil {
		return liveguard.LiveManagerHistoryResult{}, false
	}
	var result liveguard.LiveManagerHistoryResult
	if err := json.Unmarshal(b, &result); err != nil {
		return liveguard.LiveManagerHistoryResult{}, false
	}
	return result, true
}

func runUniverseResearch(ctx context.Context, cfg config.Config, db *storage.DB) error {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		analysis, err = analyze(ctx, cfg, db)
		if err != nil {
			return fmt.Errorf("build latest analysis for universe research: %w", err)
		}
	}
	btc1d, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return fmt.Errorf("load BTC benchmark for universe research: %w", err)
	}
	assets := map[string][]market.Candle{}
	for _, symbol := range agent2.ResearchUniverseSymbols(cfg) {
		candles, err := db.LoadCandles(symbol, "1d", cfg.Data.CandleLimit)
		if err != nil {
			assets[symbol] = nil
			continue
		}
		assets[symbol] = candles
	}
	report := agent2.BuildUniverseResearchReport(cfg, analysis, assets, btc1d, time.Now())
	if err := writeUniverseResearchReport(report); err != nil {
		return err
	}
	fmt.Println(universeResearchMarkdown(report))
	return nil
}

func runExportTraining(cfg config.Config, db *storage.DB) error {
	btc, err := loadBTC(cfg, db)
	if err != nil {
		return err
	}
	assets := map[string][]market.Candle{}
	for _, sym := range cfg.Data.Symbols.Assets {
		candles, err := db.LoadCandles(sym, "1d", cfg.Data.CandleLimit)
		if err != nil {
			continue
		}
		assets[sym] = candles
	}
	result, err := backtest.BuildTrainingDataset(cfg, btc, assets, "data/training", backtest.TrainingDatasetConfig{})
	if err != nil {
		return err
	}
	fmt.Println(result.Summary)
	return nil
}
