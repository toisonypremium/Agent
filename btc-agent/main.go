package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/aiagent"
	"btc-agent/internal/aieval"
	"btc-agent/internal/backtest"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/learning"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/market"
	"btc-agent/internal/notify"
	"btc-agent/internal/reportio"
	"btc-agent/internal/research"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
)

var telegramManager = notify.NewTelegramManager("reports", nil)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return usage()
	}
	cmd := args[1]
	cfgPath := "config.yaml"
	for i := 2; i < len(args)-1; i++ {
		if args[i] == "--config" {
			cfgPath = args[i+1]
		}
	}
	if cmd == "eval-ai" {
		return runAIEvaluation()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer db.Close()

	switch cmd {
	case "fetch":
		return fetch(ctx, cfg, db)
	case "analyze":
		_, err := analyze(ctx, cfg, db)
		return err
	case "plan":
		_, err := plan(ctx, cfg, db)
		return err
	case "run-daily":
		return runDaily(ctx, cfg, db)
	case "status":
		status, err := formatStatus(db)
		if err != nil {
			return err
		}
		fmt.Println(status)
		return nil
	case "backtest":
		return runBacktest(cfg, db)
	case "backtest-live-manager":
		researchExpiryDays, err := intArgValue(args, "--research-expiry-days")
		if err != nil {
			return err
		}
		researchHoldPriceAboveDiscountPct, err := floatArgValue(args, "--research-hold-if-price-above-discount-pct")
		if err != nil {
			return err
		}
		return runBacktestLiveManager(cfg, db, hasFlag(args, "--research-armed"), argValue(args, "--research-profile"), researchExpiryDays, hasFlag(args, "--research-hold-through-watch"), researchHoldPriceAboveDiscountPct, hasFlag(args, "--production-armed-probe"))
	case "learn":
		return runLearning(cfg, db)
	case "export-training":
		return runExportTraining(cfg, db)
	case "run-ai-watch":
		return runAIWatch(ctx, cfg, db)
	case "live-proof":
		return runLiveProof(ctx, cfg, db)
	case "live-readiness":
		return runLiveReadiness(ctx, cfg, db)
	case "live-doctor":
		_, err := runLiveDoctor(ctx, cfg, db)
		return err
	case "research-doctor":
		_, err := runResearchDoctor(ctx, cfg)
		return err
	case "research-brief":
		_, err := runResearchBrief(ctx, cfg, true)
		return err
	case "execute-live-proof-order":
		return runExecuteLiveProofOrder(ctx, cfg, db, argValue(args, "--confirm"))
	case "auto-live-order":
		return runAutoLiveOrder(ctx, cfg, db, hasFlag(args, "--dry-run"))
	case "cancel-all-live-orders":
		return runCancelAllLiveOrders(ctx, cfg, db, hasFlag(args, "--dry-run"))
	case "simulate-live-manager":
		return runSimulateLiveManager(cfg)
	case "operator-halt":
		return runOperatorHalt(db)
	case "operator-resume":
		return runOperatorResume(db)
	case "operator-status":
		return runOperatorStatus(db)
	case "reconcile-live-orders":
		return runReconcileLiveOrders(ctx, cfg, db)
	case "live-positions":
		return runLivePositions(cfg, db)
	case "live-supervisor":
		_, err := runLiveSupervisorCycle(ctx, cfg, db, &liveSupervisorState{}, hasFlag(args, "--dry-run"))
		return err
	case "maintenance":
		return runMaintenance(cfg, db)
	case "scheduler":
		return runScheduler(ctx, cfg, db, hasFlag(args, "--run-now"), hasFlag(args, "--dry-run"))
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: btc-agent <fetch|analyze|plan|run-daily|run-ai-watch|backtest|backtest-live-manager|learn|export-training|eval-ai|live-proof|live-readiness|live-doctor|research-doctor|research-brief|execute-live-proof-order|auto-live-order|live-supervisor|cancel-all-live-orders|simulate-live-manager|operator-halt|operator-resume|operator-status|reconcile-live-orders|live-positions|maintenance|status|scheduler> --config config.yaml [--run-now|--dry-run|--research-armed|--production-armed-probe|--research-profile <name>|--research-expiry-days <days>|--research-hold-through-watch|--research-hold-if-price-above-discount-pct <pct>]")
}

func fetch(ctx context.Context, cfg config.Config, db *storage.DB) error {
	client := exchange.NewBinance(cfg.Data.BinanceBaseURL)
	symbols := append([]string{cfg.Data.Symbols.BTC}, cfg.Data.Symbols.Assets...)
	tasks := []fetchTask{}
	for _, sym := range symbols {
		for _, interval := range cfg.Data.Intervals {
			tasks = append(tasks, fetchTask{Symbol: sym, Interval: interval})
		}
	}

	workerCount := 4
	if len(tasks) < workerCount {
		workerCount = len(tasks)
	}
	if workerCount < 1 {
		return nil
	}

	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan fetchTask)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				if err := fetchOne(fetchCtx, cfg, db, client, task); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
				}
			}
		}()
	}
dispatch:
	for _, task := range tasks {
		select {
		case <-fetchCtx.Done():
			break dispatch
		case jobs <- task:
		}
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

type fetchTask struct {
	Symbol   string
	Interval string
}

func fetchOne(ctx context.Context, cfg config.Config, db *storage.DB, client *exchange.BinanceClient, task fetchTask) error {
	limit := cfg.Data.CandleLimit
	mode := "cold"
	latest, found, err := db.LatestCandleOpenTime(task.Symbol, task.Interval)
	if err != nil {
		return fmt.Errorf("latest candle %s %s: %w", task.Symbol, task.Interval, err)
	}
	if found {
		mode = "incremental"
		limit = 20
		if cfg.Data.CandleLimit > 0 && cfg.Data.CandleLimit < limit {
			limit = cfg.Data.CandleLimit
		}
	}
	candles, err := client.Klines(ctx, task.Symbol, task.Interval, limit)
	if err != nil {
		return fmt.Errorf("fetch %s %s: %w", task.Symbol, task.Interval, err)
	}
	toSave := candles
	if found {
		toSave = toSave[:0]
		for _, candle := range candles {
			if !candle.OpenTime.Before(latest) {
				toSave = append(toSave, candle)
			}
		}
	}
	if len(toSave) > 0 {
		if err := db.SaveCandles(toSave); err != nil {
			return fmt.Errorf("save candles %s %s: %w", task.Symbol, task.Interval, err)
		}
	}
	log.Printf("fetch candles %s %s mode=%s fetched=%d saved=%d", task.Symbol, task.Interval, mode, len(candles), len(toSave))
	return nil
}

func loadBTC(cfg config.Config, db *storage.DB) (map[string][]market.Candle, error) {
	out := map[string][]market.Candle{}
	for _, interval := range cfg.Data.Intervals {
		candles, err := db.LoadCandles(cfg.Data.Symbols.BTC, interval, cfg.Data.CandleLimit)
		if err != nil {
			return nil, err
		}
		out[interval] = candles
	}
	return out, nil
}

func loadAssets(cfg config.Config, db *storage.DB) (map[string][]market.Candle, error) {
	out := map[string][]market.Candle{}
	for _, sym := range cfg.Data.Symbols.Assets {
		candles, err := db.LoadCandles(sym, "1d", cfg.Data.CandleLimit)
		if err != nil {
			return nil, err
		}
		out[sym] = candles
	}
	return out, nil
}

func analyze(ctx context.Context, cfg config.Config, db *storage.DB) (agent1.MarketAnalysis, error) {
	btc, err := loadBTC(cfg, db)
	if err != nil {
		return agent1.MarketAnalysis{}, err
	}
	fg, err := exchange.FetchFearGreed(ctx)
	if err != nil {
		log.Printf("feargreed warning: %v", err)
	}
	analysis, err := agent1.Analyze(cfg, btc, fg)
	if err != nil {
		return analysis, err
	}
	if err := db.SaveAnalysis(analysis); err != nil {
		return analysis, err
	}
	report := agent1.DailyReport(analysis, "Agent 2 chưa chạy trong lệnh analyze.")
	_ = db.SaveReport("daily_brief", report)
	_ = storage.SaveReportFiles("reports", analysis, agent2.Plan{ActionPermission: analysis.ActionPermission, State: agent2.StateWatch, Summary: "Agent 2 chưa chạy trong lệnh analyze."}, report)
	fmt.Println(report)
	return analysis, nil
}

func plan(ctx context.Context, cfg config.Config, db *storage.DB) (agent2.Plan, error) {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return agent2.Plan{}, fmt.Errorf("load latest analysis: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return agent2.Plan{}, err
	}
	btc1d, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return agent2.Plan{}, fmt.Errorf("load BTC benchmark for relative strength: %w", err)
	}
	benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d, "BTCUSDT": btc1d}
	p := agent2.BuildPlanWithBenchmarks(cfg, analysis, assets, benchmarks)
	if err := db.SavePlan(p); err != nil {
		return p, err
	}
	orders := agent2.OrdersFromPlan(p, cfg.Execution.OrderExpiryHours)
	if err := db.SaveOrders(orders); err != nil {
		return p, err
	}
	fmt.Println(p.JSON())
	return p, nil
}

func runDaily(ctx context.Context, cfg config.Config, db *storage.DB) error {
	return runDailyWithNotify(ctx, cfg, db, true)
}

func runDailyWithNotify(ctx context.Context, cfg config.Config, db *storage.DB, notifyTelegram bool) error {
	if err := fetch(ctx, cfg, db); err != nil {
		return err
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return err
	}
	p, err := plan(ctx, cfg, db)
	if err != nil {
		return err
	}
	report := agent1.DailyReport(analysis, agent2.Summary(p))
	if err := db.SaveReport("run_daily", report); err != nil {
		return err
	}
	if err := storage.SaveReportFiles("reports", analysis, p, report); err != nil {
		return err
	}
	// #5: auto live order in runDaily bypasses supervisor state tracking.
	// Supervisor handles auto live order execution via its own cycle.
	// runDaily only triggers if supervisor is NOT enabled (standalone daily run).
	if cfg.Live.Enabled && cfg.Live.AutoExecute && p.State == agent2.StateActiveLimit && !cfg.Live.SupervisorEnabled {
		if err := requireAutoLiveRuntime(cfg); err != nil {
			log.Printf("Automatic live order blocked: %v", err)
		} else {
			log.Println("Active limit state reached in daily run (supervisor disabled). Executing automatic canary live order placement...")
			if err := runAutoLiveOrder(ctx, cfg, db, false); err != nil {
				log.Printf("Automatic live order placement error: %v", err)
			}
		}
	}
	if notifyTelegram && cfg.Notify.Enabled {
		switch cfg.Notify.Provider {
		case "telegram":
			sendTelegram(ctx, cfg, "run-daily", telegramreport.DailyHumanText(analysis, p))
		case "ntfy":
			if err := notify.Ntfy(ctx, cfg.Notify.NtfyTopic, report); err != nil {
				log.Printf("ntfy warning: %v", err)
			}
		}
	}
	fmt.Println(report)
	return nil
}

func buildBacktestResult(cfg config.Config, db *storage.DB) (backtest.Result, error) {
	daily, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return backtest.Result{}, err
	}
	result, err := backtest.RunBTC(backtest.Config{MinWindow1D: 60, HorizonDays: []int{1, 3, 7, 14}}, daily)
	if err != nil {
		return backtest.Result{}, err
	}
	btc := map[string][]market.Candle{"1d": daily}
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
	assets := map[string][]market.Candle{}
	for _, sym := range cfg.Data.Symbols.Assets {
		candles, err := db.LoadCandles(sym, "1d", cfg.Data.CandleLimit)
		if err != nil {
			continue
		}
		assets[sym] = candles
	}
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
	daily, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return err
	}
	btc := map[string][]market.Candle{"1d": daily}
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
	result := learning.BuildRecommendations(backtestResult)
	md := learning.Markdown(result)
	if err := learning.SaveReports("reports", result, md); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func runExportTraining(cfg config.Config, db *storage.DB) error {
	daily, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
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
	result, err := backtest.BuildTrainingDataset(cfg, map[string][]market.Candle{"1d": daily}, assets, "data/training", backtest.TrainingDatasetConfig{})
	if err != nil {
		return err
	}
	fmt.Println(result.Summary)
	return nil
}

func runAIEvaluation() error {
	result, err := aieval.Run(aieval.Config{})
	if err != nil {
		return err
	}
	fmt.Println(result.Summary)
	return nil
}

func runAIWatch(ctx context.Context, cfg config.Config, db *storage.DB) error {
	if err := fetch(ctx, cfg, db); err != nil {
		return err
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return err
	}
	p, err := plan(ctx, cfg, db)
	if err != nil {
		return err
	}
	status, _ := formatStatus(db)
	snap := aiagent.Snapshot{Analysis: analysis, Plan: p, Status: status}
	var caller aiagent.JSONCaller
	if cfg.AI.Enabled {
		client, err := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, cfg.AI.MaxTokens, cfg.AI.Temperature)
		if err != nil {
			log.Printf("ai warning: %v", err)
		} else {
			caller = client
		}
	}
	report, err := aiagent.Generate(ctx, caller, snap)
	if err != nil {
		log.Printf("ai report warning: %v", err)
	}
	if err := db.SaveReport("ai_watch", report.TelegramText); err != nil {
		return err
	}
	if err := saveJSONFile("reports", "ai_watch_latest.json", report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "ai_watch_latest.md"), []byte(report.TelegramText), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && (!cfg.AI.Enabled || cfg.AI.TelegramEnabled) {
		sendTelegram(ctx, cfg, "run-ai-watch", report.TelegramText)
	}
	fmt.Println(report.TelegramText)
	return nil
}

func requireAutoLiveRuntime(cfg config.Config) error {
	if os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") != "true" {
		return fmt.Errorf("BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	}
	if !cfg.Live.Enabled {
		return fmt.Errorf("live.enabled=false")
	}
	if !cfg.Live.AutoExecute {
		return fmt.Errorf("live.auto_execute=false")
	}
	if cfg.Live.RequireManualConfirm {
		return fmt.Errorf("live.require_manual_confirm=true")
	}
	if !cfg.Live.CanaryMode {
		return fmt.Errorf("live.canary_mode=true required for auto live execution")
	}
	if cfg.Live.ProofOnly {
		return fmt.Errorf("live.proof_only=true")
	}
	if !cfg.Execution.RealTradingEnabled {
		return fmt.Errorf("execution.real_trading_enabled=false")
	}
	return nil
}

func runLiveProof(ctx context.Context, cfg config.Config, db *storage.DB) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	if cfg.Live.Enabled && strings.ToLower(cfg.Live.Exchange) == "okx" {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err == nil {
			balanceReader = client
			filterReader = client
		}
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	if err := saveJSONFile("reports", "live_proof_latest.json", proof); err != nil {
		return err
	}
	md := fmt.Sprintf("LIVE TRADING READINESS PROOF\n\nStatus: %s\nSummary: %s\nNo real order was placed.\n", proof.Status, proof.Summary)
	if proof.Account.Enabled {
		md += fmt.Sprintf("Account check: auth_ok=%v balance_ok=%v base=%s free_usdt=%.2f min_required=%.2f\n", proof.Account.AuthOK, proof.Account.BalanceOK, proof.Account.BaseCurrency, proof.Account.FreeUSDT, proof.Account.MinRequiredUSDT)
		if proof.Account.Error != "" {
			md += "Account error: " + proof.Account.Error + "\n"
		}
	}
	if proof.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: enabled=%v pass=%v inst_id=%s price=%.8f qty=%.8f notional=%.2f tick=%.8f step=%.8f min_size=%.8f min_notional=%.2f\n", proof.Preflight.Enabled, proof.Preflight.Pass, proof.Preflight.InstID, proof.Preflight.Price, proof.Preflight.Quantity, proof.Preflight.Notional, proof.Preflight.TickSize, proof.Preflight.StepSize, proof.Preflight.MinSize, proof.Preflight.MinNotional)
		if len(proof.Preflight.Reasons) > 0 {
			md += "Preflight reasons: " + fmt.Sprint(proof.Preflight.Reasons) + "\n"
		}
	}
	if proof.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f post_only=%v\n", proof.Candidate.Side, proof.Candidate.Symbol, proof.Candidate.Price, proof.Candidate.Quantity, proof.Candidate.Notional, proof.Candidate.PostOnly)
	}
	if len(proof.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(proof.Reasons) + "\n"
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_proof_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "live-proof", telegramreport.LiveProofHumanText(proof))
	}
	fmt.Println(md)
	return nil
}

type liveReadinessReport struct {
	GeneratedAt                    time.Time                       `json:"generated_at"`
	Mode                           string                          `json:"mode"`
	LiveEnabled                    bool                            `json:"live_enabled"`
	RealTradingEnabled             bool                            `json:"real_trading_enabled"`
	AutoExecute                    bool                            `json:"auto_execute"`
	CanaryMode                     bool                            `json:"canary_mode"`
	CanaryMaxNotional              float64                         `json:"canary_max_notional_usdt"`
	AutoLadderEnabled              bool                            `json:"auto_ladder_enabled"`
	MaxAutoLayersPerCycle          int                             `json:"max_auto_layers_per_cycle"`
	MaxOpenLiveOrders              int                             `json:"max_open_live_orders"`
	AutoLadderMaxNotionalUSDT      float64                         `json:"auto_ladder_max_notional_usdt"`
	OrderManagementEnabled         bool                            `json:"order_management_enabled"`
	MaxAutoLayersPerAsset          int                             `json:"max_auto_layers_per_asset"`
	MaxOpenLiveOrdersPerAsset      int                             `json:"max_open_live_orders_per_asset"`
	MaxOpenLiveOrdersTotal         int                             `json:"max_open_live_orders_total"`
	MaxLiveNotionalPerOrderUSDT    float64                         `json:"max_live_notional_per_order_usdt"`
	MaxLiveNotionalPerAssetUSDT    float64                         `json:"max_live_notional_per_asset_usdt"`
	MaxLiveNotionalTotalUSDT       float64                         `json:"max_live_notional_total_usdt"`
	CancelIfPlanNotActive          bool                            `json:"cancel_if_plan_not_active"`
	CancelIfPriceAboveDiscountZone float64                         `json:"cancel_if_price_above_discount_zone_pct"`
	ReplaceIfPriceDriftPct         float64                         `json:"replace_if_price_drift_pct"`
	CancelStaleAfterMinutes        int                             `json:"cancel_stale_after_minutes"`
	RequireManualConfirm           bool                            `json:"require_manual_confirm"`
	ProofOnly                      bool                            `json:"proof_only"`
	AutoLiveEnv                    bool                            `json:"auto_live_env"`
	OperatorHalted                 bool                            `json:"operator_halted"`
	CredentialEnvPresent           map[string]bool                 `json:"credential_env_present"`
	PlanState                      agent2.State                    `json:"plan_state"`
	Proof                          liveguard.Proof                 `json:"proof"`
	LadderProof                    liveguard.LadderProof           `json:"ladder_proof"`
	OpenLiveOrders                 []live.OrderStatus              `json:"open_live_orders"`
	LivePositions                  []live.LivePosition             `json:"live_positions"`
	DataHealth                     liveguard.DataHealthResult      `json:"data_health"`
	ReconcileSafety                liveguard.ReconcileSafetyResult `json:"reconcile_safety"`
	RiskGovernor                   liveguard.RiskGovernorResult    `json:"risk_governor"`
	AutoLiveBlockers               []string                        `json:"auto_live_blockers"`
	Summary                        string                          `json:"summary"`
}

func runLiveReadiness(ctx context.Context, cfg config.Config, db *storage.DB) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	halted, err := db.IsHalted()
	if err != nil {
		return fmt.Errorf("load operator halt: %w", err)
	}
	open, err := db.OpenLiveOrders()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return fmt.Errorf("load assets for data health: %w", err)
	}
	dataHealth := liveguard.CheckDataHealth(cfg, analysis, p, assets, open, positions, time.Now())
	reconcileSafety := liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
	riskGovernor := liveguard.EvaluateRiskGovernor(cfg, analysis, p, open, positions, dataHealth, reconcileSafety)
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	if cfg.Live.Enabled && strings.ToLower(cfg.Live.Exchange) == "okx" {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err == nil {
			balanceReader = client
			filterReader = client
		}
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	ladderProof := liveguard.BuildLadderProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	report := liveReadinessReport{
		GeneratedAt:                    time.Now(),
		Mode:                           os.Getenv("BTC_AGENT_MODE"),
		LiveEnabled:                    cfg.Live.Enabled,
		RealTradingEnabled:             cfg.Execution.RealTradingEnabled,
		AutoExecute:                    cfg.Live.AutoExecute,
		CanaryMode:                     cfg.Live.CanaryMode,
		CanaryMaxNotional:              cfg.Live.CanaryMaxNotionalUSDT,
		AutoLadderEnabled:              cfg.Live.AutoLadderEnabled,
		MaxAutoLayersPerCycle:          cfg.Live.MaxAutoLayersPerCycle,
		MaxOpenLiveOrders:              cfg.Live.MaxOpenLiveOrders,
		AutoLadderMaxNotionalUSDT:      cfg.Live.AutoLadderMaxNotionalUSDT,
		OrderManagementEnabled:         cfg.Live.OrderManagementEnabled,
		MaxAutoLayersPerAsset:          cfg.Live.MaxAutoLayersPerAsset,
		MaxOpenLiveOrdersPerAsset:      cfg.Live.MaxOpenLiveOrdersPerAsset,
		MaxOpenLiveOrdersTotal:         cfg.Live.MaxOpenLiveOrdersTotal,
		MaxLiveNotionalPerOrderUSDT:    cfg.Live.MaxLiveNotionalPerOrderUSDT,
		MaxLiveNotionalPerAssetUSDT:    cfg.Live.MaxLiveNotionalPerAssetUSDT,
		MaxLiveNotionalTotalUSDT:       cfg.Live.MaxLiveNotionalTotalUSDT,
		CancelIfPlanNotActive:          cfg.Live.CancelIfPlanNotActive,
		CancelIfPriceAboveDiscountZone: cfg.Live.CancelIfPriceAboveDiscountZonePct,
		ReplaceIfPriceDriftPct:         cfg.Live.ReplaceIfPriceDriftPct,
		CancelStaleAfterMinutes:        cfg.Live.CancelStaleAfterMinutes,
		RequireManualConfirm:           cfg.Live.RequireManualConfirm,
		ProofOnly:                      cfg.Live.ProofOnly,
		AutoLiveEnv:                    os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") == "true",
		OperatorHalted:                 halted,
		CredentialEnvPresent:           liveCredentialEnvPresent(cfg),
		PlanState:                      p.State,
		Proof:                          proof,
		LadderProof:                    ladderProof,
		OpenLiveOrders:                 open,
		LivePositions:                  positions,
		DataHealth:                     dataHealth,
		ReconcileSafety:                reconcileSafety,
		RiskGovernor:                   riskGovernor,
	}
	if err := requireAutoLiveRuntime(cfg); err != nil {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, err.Error())
	}
	if halted {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "operator halt active")
	}
	if len(open) > 0 && !cfg.Live.OrderManagementEnabled {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "open live order exists")
	}
	if dataHealth.Status == liveguard.DataHealthBlock {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "data health block")
	}
	if riskGovernor.Status == liveguard.RiskGovernorBlock {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "risk governor block")
	}
	report.AutoLiveBlockers = uniqueStringsMain(report.AutoLiveBlockers)
	report.Summary = liveReadinessSummary(report)
	if err := saveJSONFile("reports", "live_readiness_latest.json", report); err != nil {
		return err
	}
	md := liveReadinessMarkdown(report)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_readiness_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "live-readiness", telegramreport.LiveReadinessHumanText(liveReadinessTelegramView(report)))
	}
	fmt.Println(md)
	return nil
}

func liveCredentialEnvPresent(cfg config.Config) map[string]bool {
	out := map[string]bool{}
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env == "" {
			continue
		}
		out[env] = os.Getenv(env) != ""
	}
	return out
}

func runLiveDoctor(ctx context.Context, cfg config.Config, db *storage.DB) (liveguard.RuntimeDoctorResult, error) {
	result := buildLiveDoctorResult(ctx, cfg, db)
	if err := writeLiveDoctorResult(result); err != nil {
		return result, err
	}
	fmt.Println(liveDoctorMarkdown(result))
	return result, nil
}

func writeLiveDoctorResult(result liveguard.RuntimeDoctorResult) error {
	result.RefreshSummary()
	if err := saveJSONFile("reports", "live_doctor_latest.json", result); err != nil {
		return err
	}
	md := liveDoctorMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "live_doctor_latest.md"), []byte(md), 0600)
}

func buildLiveDoctorResult(ctx context.Context, cfg config.Config, db *storage.DB) liveguard.RuntimeDoctorResult {
	result := liveguard.RuntimeDoctorResult{GeneratedAt: time.Now(), CredentialEnvPresent: liveCredentialEnvPresent(cfg), AutoLiveEnv: os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") == "true", TelegramTokenPresent: firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN")) != "", TelegramChatPresent: firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID")) != ""}
	if !result.AutoLiveEnv && cfg.Live.Enabled && cfg.Live.AutoExecute {
		result.Blockers = append(result.Blockers, "BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	}
	missingCreds := []string{}
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env != "" && !result.CredentialEnvPresent[env] {
			missingCreds = append(missingCreds, env)
		}
	}
	if cfg.Live.Enabled && len(missingCreds) > 0 {
		result.Blockers = append(result.Blockers, "missing OKX credential env: "+strings.Join(missingCreds, ", "))
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && (!result.TelegramTokenPresent || !result.TelegramChatPresent) {
		result.Warnings = append(result.Warnings, "telegram token/chat missing; notifications will be skipped")
	}
	if halted, err := db.IsHalted(); err != nil {
		result.Blockers = append(result.Blockers, "read operator halt: "+err.Error())
	} else {
		result.OperatorHalted = halted
		if halted {
			result.Blockers = append(result.Blockers, "operator halt active")
		}
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		result.Blockers = append(result.Blockers, "load open live orders: "+err.Error())
	} else {
		result.OpenLiveOrders = len(open)
	}
	plan, planErr := db.LatestPlan()
	if planErr != nil {
		result.Warnings = append(result.Warnings, "latest plan unavailable: "+planErr.Error())
	} else {
		result.PlanState = plan.State
	}
	analysis, analysisErr := db.LatestAnalysis()
	if analysisErr != nil {
		result.Warnings = append(result.Warnings, "latest analysis unavailable: "+analysisErr.Error())
	}
	positions, positionsErr := db.LivePositions()
	if positionsErr != nil {
		result.Blockers = append(result.Blockers, "load live positions: "+positionsErr.Error())
	}
	if planErr == nil && analysisErr == nil && err == nil && positionsErr == nil {
		assets, assetsErr := loadAssets(cfg, db)
		if assetsErr != nil {
			result.Warnings = append(result.Warnings, "load assets for data health: "+assetsErr.Error())
		} else {
			result.DataHealth = liveguard.CheckDataHealth(cfg, analysis, plan, assets, open, positions, time.Now())
			result.ReconcileSafety = liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
			result.RiskGovernor = liveguard.EvaluateRiskGovernor(cfg, analysis, plan, open, positions, result.DataHealth, result.ReconcileSafety)
			if result.DataHealth.Status == liveguard.DataHealthBlock {
				result.Blockers = append(result.Blockers, result.DataHealth.Blockers...)
			}
			if result.ReconcileSafety.Status == liveguard.ReconcileBlock {
				result.Blockers = append(result.Blockers, result.ReconcileSafety.Blockers...)
			}
			if result.RiskGovernor.Status == liveguard.RiskGovernorBlock {
				result.Blockers = append(result.Blockers, result.RiskGovernor.Blockers...)
			}
		}
	}
	if cfg.Live.Enabled && len(missingCreds) == 0 {
		client, clientErr := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if clientErr != nil {
			result.Blockers = append(result.Blockers, "create OKX client: "+clientErr.Error())
		} else {
			result.OKXClientReady = true
			if planErr == nil {
				proof := liveguard.BuildProofWithChecks(ctx, cfg, plan, client, client)
				result.OKXReadOnlyChecked = proof.Account.Enabled || proof.Preflight.Enabled
				result.ProofStatus = proof.Status
				result.AccountAuthOK = proof.Account.AuthOK
				result.AccountBalanceOK = proof.Account.BalanceOK
				result.PreflightPass = proof.Preflight.Pass
				if proof.Status == liveguard.NotReadyBalance || proof.Status == liveguard.NotReadyFilters || proof.Status == liveguard.NotReadyConfig {
					result.Blockers = append(result.Blockers, proof.Reasons...)
				}
			}
		}
	}
	result.RefreshSummary()
	return result
}

func liveDoctorMarkdown(result liveguard.RuntimeDoctorResult) string {
	md := fmt.Sprintf("LIVE DOCTOR\n\nGenerated: %s\nStatus: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Summary)
	md += fmt.Sprintf("Auto live env: %v\n", result.AutoLiveEnv)
	md += "OKX credential env present:\n"
	for _, env := range []string{"OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"} {
		if _, ok := result.CredentialEnvPresent[env]; ok {
			md += fmt.Sprintf("- %s: %v\n", env, result.CredentialEnvPresent[env])
		}
	}
	md += fmt.Sprintf("Telegram env/config present: token=%v chat=%v\n", result.TelegramTokenPresent, result.TelegramChatPresent)
	md += fmt.Sprintf("Operator halt: %v\n", result.OperatorHalted)
	md += fmt.Sprintf("Open live orders: %d\n", result.OpenLiveOrders)
	md += fmt.Sprintf("Plan state: %s\n", result.PlanState)
	md += fmt.Sprintf("OKX client ready: %v | read-only checked: %v\n", result.OKXClientReady, result.OKXReadOnlyChecked)
	if result.ProofStatus != "" {
		md += fmt.Sprintf("Proof status: %s | auth=%v balance=%v preflight=%v\n", result.ProofStatus, result.AccountAuthOK, result.AccountBalanceOK, result.PreflightPass)
	}
	if result.DataHealth.Status != "" {
		md += fmt.Sprintf("Data health: %s | %s\n", result.DataHealth.Status, result.DataHealth.Summary)
	}
	if result.ReconcileSafety.Status != "" {
		md += fmt.Sprintf("Reconcile safety: %s | %s\n", result.ReconcileSafety.Status, result.ReconcileSafety.Summary)
	}
	if result.RiskGovernor.Status != "" {
		md += fmt.Sprintf("Risk governor: %s | %s\n", result.RiskGovernor.Status, result.RiskGovernor.Summary)
	}
	if len(result.Blockers) > 0 {
		md += "\nBlockers:\n"
		for _, blocker := range result.Blockers {
			md += "- " + blocker + "\n"
		}
	}
	if len(result.Warnings) > 0 {
		md += "\nWarnings:\n"
		for _, warning := range result.Warnings {
			md += "- " + warning + "\n"
		}
	}
	md += "\nSafety: spot limit BUY post-only only; no futures, no leverage, no market order.\n"
	return md
}

func liveReadinessTelegramView(r liveReadinessReport) telegramreport.LiveReadinessView {
	return telegramreport.LiveReadinessView{
		GeneratedAt:                   r.GeneratedAt,
		Mode:                          r.Mode,
		AutoLiveEnv:                   r.AutoLiveEnv,
		OperatorHalted:                r.OperatorHalted,
		CredentialEnvPresent:          r.CredentialEnvPresent,
		PlanState:                     r.PlanState,
		Proof:                         r.Proof,
		OpenLiveOrders:                len(r.OpenLiveOrders),
		LivePositions:                 len(r.LivePositions),
		DataHealth:                    r.DataHealth,
		ReconcileSafety:               r.ReconcileSafety,
		RiskGovernor:                  r.RiskGovernor,
		AutoLiveBlockers:              r.AutoLiveBlockers,
		LiveEnabled:                   r.LiveEnabled,
		RealTradingEnabled:            r.RealTradingEnabled,
		AutoExecute:                   r.AutoExecute,
		CanaryMode:                    r.CanaryMode,
		CanaryMaxNotional:             r.CanaryMaxNotional,
		RequireManualConfirm:          r.RequireManualConfirm,
		ProofOnly:                     r.ProofOnly,
		AutoLadderEnabled:             r.AutoLadderEnabled,
		MaxAutoLayers:                 r.MaxAutoLayersPerCycle,
		MaxOpenLiveOrders:             r.MaxOpenLiveOrders,
		AutoLadderMaxNotional:         r.AutoLadderMaxNotionalUSDT,
		OrderManagementEnabled:        r.OrderManagementEnabled,
		MaxAutoLayersPerAsset:         r.MaxAutoLayersPerAsset,
		MaxOpenLiveOrdersPerAsset:     r.MaxOpenLiveOrdersPerAsset,
		MaxOpenLiveOrdersTotal:        r.MaxOpenLiveOrdersTotal,
		MaxLiveNotionalPerOrderUSDT:   r.MaxLiveNotionalPerOrderUSDT,
		MaxLiveNotionalPerAssetUSDT:   r.MaxLiveNotionalPerAssetUSDT,
		MaxLiveNotionalTotalUSDT:      r.MaxLiveNotionalTotalUSDT,
		CancelIfPlanNotActive:         r.CancelIfPlanNotActive,
		CancelIfPriceAboveDiscountPct: r.CancelIfPriceAboveDiscountZone,
		ReplaceIfPriceDriftPct:        r.ReplaceIfPriceDriftPct,
		CancelStaleAfterMinutes:       r.CancelStaleAfterMinutes,
		LadderProof:                   r.LadderProof,
	}
}

func liveReadinessSummary(r liveReadinessReport) string {
	if len(r.AutoLiveBlockers) == 0 && r.Proof.Status == liveguard.ReadyForManualLiveProofOrder {
		return "LIVE_READY_FOR_CANARY_AUTO"
	}
	if r.Proof.Status == liveguard.ReadyForManualLiveProofOrder {
		return fmt.Sprintf("LIVE_READY_FOR_MANUAL; auto_blockers=%d", len(r.AutoLiveBlockers))
	}
	return fmt.Sprintf("LIVE_NOT_READY; proof=%s auto_blockers=%d", r.Proof.Status, len(r.AutoLiveBlockers))
}

func liveReadinessMarkdown(r liveReadinessReport) string {
	md := fmt.Sprintf("LIVE READINESS REPORT\n\nGenerated: %s\nSummary: %s\n\n", r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), r.Summary)
	md += fmt.Sprintf("Mode: %s | auto env: %v\n", emptyDefault(r.Mode, "unset"), r.AutoLiveEnv)
	md += fmt.Sprintf("Config: live=%v real=%v auto=%v canary=%v canary_max=%.2f manual_confirm=%v proof_only=%v\n", r.LiveEnabled, r.RealTradingEnabled, r.AutoExecute, r.CanaryMode, r.CanaryMaxNotional, r.RequireManualConfirm, r.ProofOnly)
	md += fmt.Sprintf("Legacy auto ladder: enabled=%v max_layers_per_cycle=%d max_open_orders_legacy=%d max_notional=%.2f proof=%s candidates=%d total=%.2f\n", r.AutoLadderEnabled, r.MaxAutoLayersPerCycle, r.MaxOpenLiveOrders, r.AutoLadderMaxNotionalUSDT, r.LadderProof.Status, len(r.LadderProof.Candidates), r.LadderProof.TotalNotional)
	md += fmt.Sprintf("Managed order engine: enabled=%v max_layers_per_asset=%d max_open_per_asset=%d max_open_total=%d\n", r.OrderManagementEnabled, r.MaxAutoLayersPerAsset, r.MaxOpenLiveOrdersPerAsset, r.MaxOpenLiveOrdersTotal)
	md += fmt.Sprintf("Managed notional caps: per_order=%.2f per_asset=%.2f total=%.2f USDT\n", r.MaxLiveNotionalPerOrderUSDT, r.MaxLiveNotionalPerAssetUSDT, r.MaxLiveNotionalTotalUSDT)
	md += fmt.Sprintf("Managed cancel/replace: cancel_plan_inactive=%v cancel_price_above_discount=%.2f%% replace_drift=%.2f%% stale_after=%dm\n", r.CancelIfPlanNotActive, r.CancelIfPriceAboveDiscountZone*100, r.ReplaceIfPriceDriftPct*100, r.CancelStaleAfterMinutes)
	if r.CanaryMode && r.OrderManagementEnabled {
		md += "Risk sizing: BTC permission controls budget multiplier; hard safety still blocks dangerous actions.\n"
		md += "Opportunity allocation: live capital follows current setup score, not fixed portfolio percentages.\n"
		md += "Quality multiplier: A/B full, C reduced, NO_SAMPLE/missing probe, D blocked.\n"
	}
	md += fmt.Sprintf("Operator halt: %v\n", r.OperatorHalted)
	md += "Credential env present:\n"
	for _, env := range []string{"OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"} {
		if _, ok := r.CredentialEnvPresent[env]; ok {
			md += fmt.Sprintf("- %s: %v\n", env, r.CredentialEnvPresent[env])
		}
	}
	md += fmt.Sprintf("Plan state: %s\n", r.PlanState)
	md += fmt.Sprintf("Proof: %s | %s\n", r.Proof.Status, r.Proof.Summary)
	md += fmt.Sprintf("Open live orders: %d\n", len(r.OpenLiveOrders))
	md += fmt.Sprintf("Live positions: %d\n", len(r.LivePositions))
	md += fmt.Sprintf("Data health: %s | %s\n", r.DataHealth.Status, r.DataHealth.Summary)
	md += fmt.Sprintf("Reconcile safety: %s | %s\n", r.ReconcileSafety.Status, r.ReconcileSafety.Summary)
	md += fmt.Sprintf("Risk governor: %s | %s\n", r.RiskGovernor.Status, r.RiskGovernor.Summary)
	if len(r.DataHealth.Blockers) > 0 {
		md += "Data health blockers:\n"
		for _, reason := range r.DataHealth.Blockers {
			md += "- " + reason + "\n"
		}
	}
	if len(r.RiskGovernor.Blockers) > 0 {
		md += "Risk governor blockers:\n"
		for _, reason := range r.RiskGovernor.Blockers {
			md += "- " + reason + "\n"
		}
	}
	if r.Proof.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f canary=%v\n", r.Proof.Candidate.Side, r.Proof.Candidate.Symbol, r.Proof.Candidate.Price, r.Proof.Candidate.Quantity, r.Proof.Candidate.Notional, r.Proof.Candidate.Canary)
	}
	if r.Proof.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: pass=%v inst_id=%s notional=%.2f reasons=%v\n", r.Proof.Preflight.Pass, r.Proof.Preflight.InstID, r.Proof.Preflight.Notional, r.Proof.Preflight.Reasons)
	}
	if len(r.AutoLiveBlockers) > 0 {
		md += "\nAuto live blockers:\n"
		for _, reason := range r.AutoLiveBlockers {
			md += "- " + reason + "\n"
		}
	}
	md += "\nNo order was placed.\n"
	return md
}

func emptyDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func uniqueStringsMain(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func runResearchDoctor(ctx context.Context, cfg config.Config) (research.DoctorResult, error) {
	result := research.RunDoctor(ctx, cfg)
	if err := saveJSONFile("reports", "research_doctor_latest.json", result); err != nil {
		return result, err
	}
	md := research.DoctorMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join("reports", "research_doctor_latest.md"), []byte(md), 0600); err != nil {
		return result, err
	}
	fmt.Println(md)
	return result, nil
}

func runResearchBrief(ctx context.Context, cfg config.Config, notifyTelegram bool) (research.BriefResult, error) {
	result := research.BuildBrief(ctx, cfg)
	if err := saveJSONFile("reports", "research_brief_latest.json", result); err != nil {
		return result, err
	}
	md := research.BriefMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join("reports", "research_brief_latest.md"), []byte(md), 0600); err != nil {
		return result, err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		telegramText := buildResearchTelegramText(ctx, cfg, result)
		sendTelegram(ctx, cfg, "research-brief", telegramText)
	}
	fmt.Println(md)
	return result, nil
}

// buildResearchTelegramText tries AI analysis first; falls back to deterministic formatter.
func buildResearchTelegramText(ctx context.Context, cfg config.Config, result research.BriefResult) string {
	if cfg.AI.Enabled && len(result.Items) > 0 {
		// Research brief needs enough room for JSON wrapper + expert Telegram text.
		// Use full 2000-token cap to avoid truncated JSON from 9router Agent.
		maxTokens := cfg.AI.MaxTokens
		if maxTokens < 2000 {
			maxTokens = 2000
		}
		llmClient, err := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, maxTokens, cfg.AI.Temperature)
		if err != nil {
			log.Printf("research ai client: %v — using deterministic formatter", err)
		} else {
			aiText, err := research.AnalyzeBriefWithAI(ctx, llmClient, result)
			if err != nil {
				log.Printf("research ai analysis: %v — using deterministic formatter", err)
			} else if aiText != "" {
				log.Printf("research brief: AI analysis ok (%d chars)", len(aiText))
				return aiText
			}
		}
	}
	return telegramreport.ResearchBriefHumanText(result)
}

func runExecuteLiveProofOrder(ctx context.Context, cfg config.Config, db *storage.DB, confirm string) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	var placer liveguard.OrderPlacer
	if err == nil {
		balanceReader = client
		filterReader = client
		placer = client
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	result := liveguard.ExecuteManualProofOrder(ctx, cfg, proof, confirm, placer, db)
	if result.Status == liveguard.LiveOrderSubmitted {
		if err := db.SaveLiveOrderFromParams(
			result.Order.ClientOrderID,
			result.Order.OrderID,
			result.Order.InstID,
			result.Candidate.Symbol,
			result.Candidate.Side,
			result.Candidate.Type,
			result.Candidate.Price,
			result.Candidate.Quantity,
			result.Candidate.Notional,
			live.StatusLiveOpen,
		); err != nil {
			return fmt.Errorf("save live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(live.OrderStatus{
			ClientOrderID: result.Order.ClientOrderID,
			OrderID:       result.Order.OrderID,
			InstID:        result.Order.InstID,
			Status:        live.StatusLiveOpen,
		}); err != nil {
			return fmt.Errorf("save live order event: %w", err)
		}
	}
	if err := saveJSONFile("reports", "live_order_proof_latest.json", result); err != nil {
		return err
	}
	md := liveOrderMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_order_proof_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "manual-live-order", telegramreport.LiveOrderHumanText(result, false))
	}
	fmt.Println(md)
	return nil
}

func runAutoLiveOrder(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool) error {
	return runAutoLiveOrderWithNotify(ctx, cfg, db, dryRun, true)
}

func runAutoLiveOrderWithNotify(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool, notifyTelegram bool) error {
	if err := requireAutoLiveRuntime(cfg); err != nil {
		return err
	}
	p, err := refreshDeterministicPlanForLive(ctx, cfg, db)
	if err != nil {
		return err
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	if len(open) > 0 || cfg.Live.AutoLadderEnabled || cfg.Live.OrderManagementEnabled {
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, notifyTelegram && !dryRun); err != nil {
			return fmt.Errorf("pre-auto reconcile live orders: %w", err)
		}
		open, err = db.OpenLiveOrdersDetailed()
		if err != nil {
			return fmt.Errorf("reload open live orders after reconcile: %w", err)
		}
	}
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis for safety gates: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return fmt.Errorf("load assets for safety gates: %w", err)
	}
	dataHealth := liveguard.CheckDataHealth(cfg, analysis, p, assets, open, positions, time.Now())
	reconcileSafety := liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
	riskGovernor := liveguard.EvaluateRiskGovernor(cfg, analysis, p, open, positions, dataHealth, reconcileSafety)
	if dataHealth.Status == liveguard.DataHealthBlock || reconcileSafety.Status == liveguard.ReconcileBlock || riskGovernor.Status == liveguard.RiskGovernorBlock {
		result := liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleBlocked, PlanState: p.State, Desired: []liveguard.ManagedDesiredOrder{}, DryRun: dryRun, DataHealth: dataHealth, ReconcileSafety: reconcileSafety, RiskGovernor: riskGovernor}
		result.Reasons = append(result.Reasons, dataHealth.Blockers...)
		result.Reasons = append(result.Reasons, reconcileSafety.Blockers...)
		result.Reasons = append(result.Reasons, riskGovernor.Blockers...)
		result.Reasons = uniqueStringsMain(result.Reasons)
		result.PerCoin = liveguard.BuildManagedCoinSummaries(cfg, p, open, result)
		result.Summary = result.Status + ": " + strings.Join(result.Reasons, "; ")
		return writeAutoLiveManagementResult(ctx, cfg, db, result, notifyTelegram && !dryRun)
	}
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	var placer liveguard.OrderPlacer
	var canceler liveguard.OrderCanceler
	if err == nil {
		balanceReader = client
		filterReader = client
		placer = client
		canceler = client
	}
	if cfg.Live.OrderManagementEnabled {
		// #9: skip OKX InstrumentFilters HTTP call when plan is not ACTIVE_LIMIT
		// and there are no open orders to cancel. Avoids unnecessary API usage every cycle.
		noActiveWork := p.State != agent2.StateActiveLimit && len(open) == 0
		filters := []live.InstrumentFilter{}
		if filterReader != nil && !noActiveWork {
			filters, err = filterReader.InstrumentFilters(ctx)
			if err != nil {
				return fmt.Errorf("load instrument filters for order management: %w", err)
			}
		}
		result := liveguard.ManageLiveOrdersDryRun(ctx, cfg, p, open, positions, filters, placer, canceler, db, dryRun)
		result.DataHealth = dataHealth
		result.ReconcileSafety = reconcileSafety
		result.RiskGovernor = riskGovernor
		if !dryRun {
			if err := persistManagedCycleResult(db, result); err != nil {
				return err
			}
		}
		if !dryRun && result.Status != liveguard.ManagedCycleBlocked && (len(result.Canceled) > 0 || len(result.Placed) > 0 || len(result.Replaced) > 0) {
			if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
				log.Printf("post-managed auto reconcile warning: %v", err)
			}
		}
		return writeAutoLiveManagementResult(ctx, cfg, db, result, notifyTelegram && !dryRun)
	}
	if cfg.Live.AutoLadderEnabled {
		proof := liveguard.BuildLadderProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
		result := liveguard.ExecuteAutoLadderProofOrder(ctx, cfg, proof, placer, open, positions, db)
		for i, order := range result.Orders {
			if !order.Submitted || i >= len(result.Candidates) {
				continue
			}
			candidate := result.Candidates[i]
			if err := db.SaveLiveOrderFromParams(order.ClientOrderID, order.OrderID, order.InstID, candidate.Symbol, candidate.Side, candidate.Type, candidate.Price, candidate.Quantity, candidate.Notional, live.StatusLiveOpen); err != nil {
				return fmt.Errorf("save auto ladder live order: %w", err)
			}
			if err := db.SaveLiveOrderEvent(live.OrderStatus{ClientOrderID: order.ClientOrderID, OrderID: order.OrderID, InstID: order.InstID, Status: live.StatusLiveOpen}); err != nil {
				return fmt.Errorf("save auto ladder live order event: %w", err)
			}
		}
		if result.Status == liveguard.LiveOrderSubmitted {
			if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
				log.Printf("post-auto ladder reconcile warning: %v", err)
			}
		}
		if err := saveJSONFile("reports", "auto_live_ladder_latest.json", result); err != nil {
			return err
		}
		md := autoLiveLadderMarkdown(result)
		if err := os.MkdirAll("reports", 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join("reports", "auto_live_ladder_latest.md"), []byte(md), 0600); err != nil {
			return err
		}
		if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
			sendTelegram(ctx, cfg, "auto-live-ladder", telegramreport.LiveLadderOrderHumanText(result))
		}
		fmt.Println(md)
		return nil
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	result := liveguard.ExecuteAutoProofOrder(ctx, cfg, proof, placer, open, positions, db)
	if result.Status == liveguard.LiveOrderSubmitted {
		if err := db.SaveLiveOrderFromParams(
			result.Order.ClientOrderID,
			result.Order.OrderID,
			result.Order.InstID,
			result.Candidate.Symbol,
			result.Candidate.Side,
			result.Candidate.Type,
			result.Candidate.Price,
			result.Candidate.Quantity,
			result.Candidate.Notional,
			live.StatusLiveOpen,
		); err != nil {
			return fmt.Errorf("save auto live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(live.OrderStatus{
			ClientOrderID: result.Order.ClientOrderID,
			OrderID:       result.Order.OrderID,
			InstID:        result.Order.InstID,
			Status:        live.StatusLiveOpen,
		}); err != nil {
			return fmt.Errorf("save auto live order event: %w", err)
		}
	}
	if result.Status == liveguard.LiveOrderSubmitted {
		if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
			log.Printf("post-auto reconcile warning: %v", err)
		}
	}
	if err := saveJSONFile("reports", "auto_live_order_latest.json", result); err != nil {
		return err
	}
	md := autoLiveOrderMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "auto_live_order_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "auto-live-order", telegramreport.LiveOrderHumanText(result, true))
	}
	fmt.Println(md)
	return nil
}

func refreshDeterministicPlanForLive(ctx context.Context, cfg config.Config, db *storage.DB) (agent2.Plan, error) {
	if err := fetch(ctx, cfg, db); err != nil {
		return agent2.Plan{}, err
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return agent2.Plan{}, err
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return agent2.Plan{}, err
	}
	btc1d, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return agent2.Plan{}, fmt.Errorf("load BTC benchmark for live plan: %w", err)
	}
	benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d, "BTCUSDT": btc1d}
	p := agent2.BuildPlanWithBenchmarks(cfg, analysis, assets, benchmarks)
	if err := db.SavePlan(p); err != nil {
		return p, err
	}
	orders := agent2.OrdersFromPlan(p, cfg.Execution.OrderExpiryHours)
	if err := db.SaveOrders(orders); err != nil {
		return p, err
	}
	return p, nil
}

type liveSupervisorState struct {
	ConsecutiveErrors int
	LastHeartbeat     time.Time
}

func runLiveSupervisorCycle(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool) (liveguard.SupervisorResult, error) {
	doctor := buildLiveDoctorResult(ctx, cfg, db)
	if err := writeLiveDoctorResult(doctor); err != nil {
		log.Printf("live doctor report warning: %v", err)
	}
	return runLiveSupervisorCycleWithDoctor(ctx, cfg, db, state, dryRun, &doctor)
}

func runLiveSupervisorCycleWithDoctor(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool, doctor *liveguard.RuntimeDoctorResult) (liveguard.SupervisorResult, error) {
	return runLiveSupervisorCycleWithDoctorNotify(ctx, cfg, db, state, dryRun, doctor, true)
}

func runLiveSupervisorCycleWithDoctorNotify(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool, doctor *liveguard.RuntimeDoctorResult, notifyTelegram bool) (liveguard.SupervisorResult, error) {
	if state == nil {
		state = &liveSupervisorState{}
	}
	result := liveguard.SupervisorResult{GeneratedAt: time.Now(), Status: liveguard.SupervisorOK, Action: liveguard.SupervisorActionManagedCycle, ConsecutiveErrors: state.ConsecutiveErrors, Doctor: doctor}
	if doctor != nil && doctor.Status == liveguard.DoctorBlock && !dryRun {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "live doctor block: "+doctor.Summary)
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "reconcile after doctor block: "+err.Error())
		}
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if !cfg.Live.SupervisorEnabled {
		result.Action = liveguard.SupervisorActionSkipped
		result.Summary = "SUPERVISOR_OK: action=skipped | live supervisor disabled"
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, result, false)
	}
	halted, err := db.IsHalted()
	if err != nil {
		state.ConsecutiveErrors++
		result.ConsecutiveErrors = state.ConsecutiveErrors
		result.Reasons = append(result.Reasons, "read operator halt: "+err.Error())
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if halted {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "operator halt active")
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "reconcile while halted: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
		}
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if dryRun {
		result.Action = liveguard.SupervisorActionHeartbeat
		if err := runAutoLiveOrderWithNotify(ctx, cfg, db, true, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "dry-run managed cycle: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
			if managed, ok := loadLatestManagedCycleReport(); ok {
				result.Managed = &managed
			}
		}
	} else {
		if err := runAutoLiveOrderWithNotify(ctx, cfg, db, false, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "managed cycle: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
			if managed, ok := loadLatestManagedCycleReport(); ok {
				result.Managed = &managed
			}
		}
	}
	if cfg.Live.AutoHaltAfterErrors > 0 && state.ConsecutiveErrors >= cfg.Live.AutoHaltAfterErrors {
		if err := db.SetHaltStatus(true); err != nil {
			result.Reasons = append(result.Reasons, "auto-halt failed: "+err.Error())
		} else {
			result.AutoHalted = true
			result.Reasons = append(result.Reasons, "auto-halt activated after repeated supervisor errors")
		}
	}
	result.RefreshSummary()
	return result, writeLiveSupervisorResult(ctx, cfg, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
}

func loadLatestManagedCycleReport() (liveguard.ManagedCycleResult, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "auto_live_management_latest.json"))
	if err != nil {
		return liveguard.ManagedCycleResult{}, false
	}
	var result liveguard.ManagedCycleResult
	if err := json.Unmarshal(b, &result); err != nil {
		return liveguard.ManagedCycleResult{}, false
	}
	return result, true
}

func writeLiveSupervisorResult(ctx context.Context, cfg config.Config, result liveguard.SupervisorResult, notifyTelegram bool) error {
	if err := saveJSONFile("reports", "live_supervisor_latest.json", result); err != nil {
		return err
	}
	md := liveSupervisorMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_supervisor_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "live-supervisor", telegramreport.LiveSupervisorHumanText(result))
	}
	fmt.Println(md)
	return nil
}

func liveSupervisorMarkdown(result liveguard.SupervisorResult) string {
	md := fmt.Sprintf("LIVE SUPERVISOR\n\nGenerated: %s\nStatus: %s\nAction: %s\nConsecutive errors: %d\nAuto halted: %v\nSummary: %s\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Action, result.ConsecutiveErrors, result.AutoHalted, result.Summary)
	if len(result.Reasons) > 0 {
		md += "Reasons:\n"
		for _, reason := range result.Reasons {
			md += "- " + reason + "\n"
		}
	}
	if result.Doctor != nil {
		md += fmt.Sprintf("\nDoctor: %s | %s\n", result.Doctor.Status, result.Doctor.Summary)
	}
	if result.Managed != nil {
		m := result.Managed
		md += fmt.Sprintf("\nManaged cycle: %s\nDesired: %d | Kept: %d | Canceled: %d | Replaced: %d | Placed: %d | Blocked: %d\n", m.Status, len(m.Desired), len(m.Kept), len(m.Canceled), len(m.Replaced), len(m.Placed), len(m.Blocked))
		if m.DataHealth.Status != "" {
			md += fmt.Sprintf("Data health: %s | %s\n", m.DataHealth.Status, m.DataHealth.Summary)
		}
		if m.ReconcileSafety.Status != "" {
			md += fmt.Sprintf("Reconcile safety: %s | %s\n", m.ReconcileSafety.Status, m.ReconcileSafety.Summary)
		}
		if m.RiskGovernor.Status != "" {
			md += fmt.Sprintf("Risk governor: %s | %s\n", m.RiskGovernor.Status, m.RiskGovernor.Summary)
		}
	}
	md += "\nSafety: spot limit BUY post-only only; no futures, no leverage, no market order.\n"
	return md
}

func shouldNotifySupervisor(cfg config.Config, result liveguard.SupervisorResult, state *liveSupervisorState) bool {
	if result.AutoHalted || result.Status == liveguard.SupervisorHalted || result.Status == liveguard.SupervisorWarn {
		return true
	}
	if result.Managed != nil && (result.Managed.Status == liveguard.ManagedCycleBlocked || result.Managed.Status == liveguard.ManagedCyclePartial || len(result.Managed.Placed) > 0 || len(result.Managed.Canceled) > 0 || len(result.Managed.Replaced) > 0 || len(result.Managed.Blocked) > 0) {
		return true
	}
	if cfg.Live.NotifyOnNoAction {
		return true
	}
	if cfg.Live.HeartbeatIntervalMinutes <= 0 || state == nil {
		return false
	}
	now := result.GeneratedAt
	if state.LastHeartbeat.IsZero() || now.Sub(state.LastHeartbeat) >= time.Duration(cfg.Live.HeartbeatIntervalMinutes)*time.Minute {
		state.LastHeartbeat = now
		return true
	}
	return false
}

func writeAutoLiveManagementResult(ctx context.Context, cfg config.Config, db *storage.DB, result liveguard.ManagedCycleResult, notifyTelegram bool) error {
	if err := db.SaveManagedCycleReport(result); err != nil {
		return fmt.Errorf("save managed cycle report: %w", err)
	}
	if err := saveJSONFile("reports", "auto_live_management_latest.json", result); err != nil {
		return err
	}
	if err := saveJSONFile("reports", "auto_live_ladder_latest.json", result); err != nil {
		return err
	}
	md := autoLiveManagementMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "auto_live_management_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "auto_live_ladder_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "auto-live-management", telegramreport.LiveOrderManagementHumanText(result))
	}
	fmt.Println(md)
	return nil
}

func persistManagedCycleResult(db *storage.DB, result liveguard.ManagedCycleResult) error {
	now := time.Now().Unix()
	for _, decision := range result.Canceled {
		status := decision.Order
		status.Status = live.StatusCanceled
		status.UpdatedAt = now
		status.LastManagementAction = "canceled: " + decision.Reason
		if err := db.SaveLiveOrderStatus(status); err != nil {
			return fmt.Errorf("save canceled live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(status); err != nil {
			return fmt.Errorf("save canceled live order event: %w", err)
		}
	}
	for _, decision := range result.Placed {
		order := decision.PlaceResult
		desired := decision.Desired
		if !order.Submitted {
			continue
		}
		meta := live.OrderStatus{LayerIndex: desired.LayerIndex, Source: desired.Source, InvalidationPrice: desired.InvalidationPrice, DecisionReason: desired.DecisionReason, LastManagementAction: "placed: " + decision.Reason, ExpiresAt: now}
		if err := db.SaveManagedLiveOrder(order.ClientOrderID, order.OrderID, order.InstID, desired.Symbol, desired.Side, desired.Type, desired.Price, desired.Quantity, desired.Notional, live.StatusLiveOpen, meta); err != nil {
			return fmt.Errorf("save managed live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(live.OrderStatus{ClientOrderID: order.ClientOrderID, OrderID: order.OrderID, InstID: order.InstID, Symbol: desired.Symbol, LayerIndex: desired.LayerIndex, Status: live.StatusLiveOpen}); err != nil {
			return fmt.Errorf("save managed live order event: %w", err)
		}
	}
	return nil
}

func autoLiveManagementMarkdown(result liveguard.ManagedCycleResult) string {
	md := fmt.Sprintf("AUTO LIVE MANAGEMENT\n\nStatus: %s\nSummary: %s\nPlan state: %s\nDry run: %v\nDesired: %d | Kept: %d | Canceled: %d | Replaced: %d | Placed: %d | Blocked: %d\n", result.Status, result.Summary, result.PlanState, result.DryRun, len(result.Desired), len(result.Kept), len(result.Canceled), len(result.Replaced), len(result.Placed), len(result.Blocked))
	if result.DataHealth.Status != "" {
		md += fmt.Sprintf("Data health: %s | %s\n", result.DataHealth.Status, result.DataHealth.Summary)
	}
	if result.ReconcileSafety.Status != "" {
		md += fmt.Sprintf("Reconcile safety: %s | %s\n", result.ReconcileSafety.Status, result.ReconcileSafety.Summary)
	}
	if result.RiskGovernor.Status != "" {
		md += fmt.Sprintf("Risk governor: %s | %s\n", result.RiskGovernor.Status, result.RiskGovernor.Summary)
	}
	appendDecision := func(title string, items []liveguard.ManagedOrderDecision) {
		if len(items) == 0 {
			return
		}
		md += "\n" + title + ":\n"
		for _, d := range items {
			md += "- " + managementDecisionLine(d) + "\n"
		}
	}
	appendDecision("Kept", result.Kept)
	appendDecision("Canceled", result.Canceled)
	appendDecision("Replaced", result.Replaced)
	appendDecision("Placed", result.Placed)
	appendDecision("Blocked", result.Blocked)
	if len(result.Reasons) > 0 {
		md += "\nReasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	if len(result.PerCoin) > 0 {
		md += "\nPER COIN\n"
		for _, coin := range result.PerCoin {
			md += fmt.Sprintf("\n%s\nState: %s\nOpen orders: %d\nDesired layers: %d\nPending notional: %.2f USDT\n", coin.Symbol, coin.State, coin.OpenOrders, coin.DesiredLayers, coin.PendingNotional)
			if len(coin.Actions) == 0 {
				md += "Actions: none\n"
			} else {
				md += "Actions:\n"
				for _, action := range coin.Actions {
					md += "- " + managementDecisionLine(action) + "\n"
				}
			}
			if len(coin.Reasons) > 0 {
				md += "Reasons: " + strings.Join(coin.Reasons, "; ") + "\n"
			}
			if len(coin.WhyNoOrder) > 0 {
				md += "Why no order: " + strings.Join(coin.WhyNoOrder, "; ") + "\n"
			}
			if coin.NextTrigger != "" {
				md += "Next trigger: " + coin.NextTrigger + "\n"
			}
		}
	}
	return md
}

func managementDecisionLine(d liveguard.ManagedOrderDecision) string {
	symbol := firstNonEmpty(d.Symbol, d.Desired.Symbol, d.Order.Symbol, live.InternalSymbol(d.Order.InstID))
	layer := firstNonZero(d.LayerIndex, d.Desired.LayerIndex, d.Order.LayerIndex)
	price := d.Desired.Price
	notional := d.Desired.Notional
	if price <= 0 {
		price = d.Order.Price
	}
	if notional <= 0 {
		notional = d.Order.Notional
	}
	out := fmt.Sprintf("%s layer=%d action=%s", symbol, layer, d.Action)
	if price > 0 {
		out += fmt.Sprintf(" @ %.8f", price)
	}
	if notional > 0 {
		out += fmt.Sprintf(" notional=%.2f", notional)
	}
	if d.Desired.AllocationTier != "" {
		out += fmt.Sprintf(" tier=%s score=%.1f", d.Desired.AllocationTier, d.Desired.AllocationScore)
	}
	if d.Desired.AllocationReason != "" {
		out += " allocation=" + d.Desired.AllocationReason
	}
	if d.Reason != "" {
		out += ": " + d.Reason
	}
	if d.Order.ClientOrderID != "" {
		out += fmt.Sprintf(" clOrdId=%s", d.Order.ClientOrderID)
	}
	if d.Error != "" {
		out += " error=" + d.Error
	}
	return out
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func autoLiveLadderMarkdown(result liveguard.LadderExecutionResult) string {
	md := fmt.Sprintf("AUTO LIVE LADDER\n\nStatus: %s\nSummary: %s\nProof status: %s\nTotal notional: %.2f\n", result.Status, result.Summary, result.ProofStatus, result.TotalNotional)
	if len(result.Candidates) > 0 {
		md += "Candidates:\n"
		for i, candidate := range result.Candidates {
			md += fmt.Sprintf("- #%d %s %s limit %.8f qty %.8f notional %.2f canary=%v\n", i+1, candidate.Side, candidate.Symbol, candidate.Price, candidate.Quantity, candidate.Notional, candidate.Canary)
		}
	}
	if len(result.Orders) > 0 {
		md += "Orders:\n"
		for i, order := range result.Orders {
			md += fmt.Sprintf("- #%d submitted=%v inst_id=%s order_id=%s client_order_id=%s\n", i+1, order.Submitted, order.InstID, order.OrderID, order.ClientOrderID)
		}
	} else {
		md += "Orders: submitted=0\n"
	}
	if len(result.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	return md
}

func liveOrderAttemptText(proof liveguard.Proof) string {
	return fmt.Sprintf("MANUAL LIVE ORDER ATTEMPT\nproof=%s symbol=%s inst_id=%s notional=%.2f\nNo order yet; hard gates still apply.", proof.Status, proof.Candidate.Symbol, proof.Preflight.InstID, proof.Candidate.Notional)
}

func autoLiveOrderMarkdown(result liveguard.ExecutionResult) string {
	canaryStr := ""
	if result.Candidate.Canary {
		canaryStr = " [CANARY MODE]"
	}
	md := fmt.Sprintf("AUTO LIVE ORDER%s\n\nStatus: %s\nSummary: %s\nProof status: %s\n", canaryStr, result.Status, result.Summary, result.ProofStatus)
	if result.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f post_only=%v\n", result.Candidate.Side, result.Candidate.Symbol, result.Candidate.Price, result.Candidate.Quantity, result.Candidate.Notional, result.Candidate.PostOnly)
	}
	if result.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: pass=%v inst_id=%s notional=%.2f\n", result.Preflight.Pass, result.Preflight.InstID, result.Preflight.Notional)
	}
	if result.Order.Submitted {
		md += fmt.Sprintf("Order: submitted=true inst_id=%s order_id=%s client_order_id=%s\n", result.Order.InstID, result.Order.OrderID, result.Order.ClientOrderID)
	} else {
		md += "Order: submitted=false\n"
	}
	if len(result.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	return md
}

func liveOrderMarkdown(result liveguard.ExecutionResult) string {
	canaryStr := ""
	if result.Candidate.Canary {
		canaryStr = " [CANARY MODE]"
	}
	md := fmt.Sprintf("MANUAL LIVE PROOF ORDER%s\n\nStatus: %s\nSummary: %s\nProof status: %s\n", canaryStr, result.Status, result.Summary, result.ProofStatus)
	if result.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f post_only=%v\n", result.Candidate.Side, result.Candidate.Symbol, result.Candidate.Price, result.Candidate.Quantity, result.Candidate.Notional, result.Candidate.PostOnly)
	}
	if result.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: pass=%v inst_id=%s notional=%.2f\n", result.Preflight.Pass, result.Preflight.InstID, result.Preflight.Notional)
	}
	if result.Order.Submitted {
		md += fmt.Sprintf("Order: submitted=true inst_id=%s order_id=%s client_order_id=%s\n", result.Order.InstID, result.Order.OrderID, result.Order.ClientOrderID)
	} else {
		md += "Order: submitted=false\n"
	}
	if len(result.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	return md
}

func runMaintenance(cfg config.Config, db *storage.DB) error {
	mcfg := storage.MaintenanceConfig{
		ReportRetentionDays:         cfg.Maintenance.ReportRetentionDays,
		EventRetentionDays:          cfg.Maintenance.EventRetentionDays,
		MaxReportFiles:              cfg.Maintenance.MaxReportFiles,
		MaxClosedPaperOrders:        cfg.Maintenance.MaxClosedPaperOrders,
		MaxCandlesPerSymbolInterval: cfg.Maintenance.MaxCandlesPerSymbolInterval,
		MaxAnalysisRows:             cfg.Maintenance.MaxAnalysisRows,
		MaxPlanRows:                 cfg.Maintenance.MaxPlanRows,
		WALCheckpoint:               cfg.Maintenance.WALCheckpoint,
	}
	if !cfg.Maintenance.Enabled {
		result := storage.MaintenanceResult{Enabled: false, GeneratedAt: time.Now(), Config: storage.NormalizeMaintenanceConfig(mcfg)}
		result.RefreshSummary()
		fmt.Println(result.Summary)
		return nil
	}
	result, err := db.PruneMaintenance(mcfg, time.Now())
	if err != nil {
		return err
	}
	deletedFiles, err := storage.PruneReportFiles("reports", result.Config.MaxReportFiles, protectedReportFiles())
	if err != nil {
		return fmt.Errorf("prune report files: %w", err)
	}
	result.ReportFilesDeleted = deletedFiles
	result.RefreshSummary()
	if err := saveJSONFile("reports", "maintenance_latest.json", result); err != nil {
		return err
	}
	md := maintenanceMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "maintenance_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func protectedReportFiles() []string {
	return []string{
		"latest.md",
		"latest.json",
		"backtest_latest.md",
		"backtest_latest.json",
		"ai_eval_latest.md",
		"ai_eval_latest.json",
		"ai_watch_latest.md",
		"ai_watch_latest.json",
		"live_proof_latest.md",
		"live_proof_latest.json",
		"live_readiness_latest.md",
		"live_readiness_latest.json",
		"live_doctor_latest.md",
		"live_doctor_latest.json",
		"research_doctor_latest.md",
		"research_doctor_latest.json",
		"research_brief_latest.md",
		"research_brief_latest.json",
		"live_order_proof_latest.md",
		"live_order_proof_latest.json",
		"live_position_latest.md",
		"live_position_latest.json",
		"live_reconcile_latest.md",
		"live_reconcile_latest.json",
		"auto_live_order_latest.md",
		"auto_live_order_latest.json",
		"auto_live_ladder_latest.md",
		"auto_live_ladder_latest.json",
		"auto_live_management_latest.md",
		"auto_live_management_latest.json",
		"live_supervisor_latest.md",
		"live_supervisor_latest.json",
		"maintenance_latest.md",
		"maintenance_latest.json",
		"learning_latest.md",
		"learning_latest.json",
		"live_manager_history_latest.md",
		"live_manager_history_latest.json",
		"live_manager_simulation_latest.md",
		"live_manager_simulation_latest.json",
		"cancel_all_live_orders_latest.md",
		"cancel_all_live_orders_latest.json",
		"telegram_state.json",
	}
}

func maintenanceMarkdown(result storage.MaintenanceResult) string {
	md := fmt.Sprintf("MAINTENANCE REPORT\n\nGenerated: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary)
	md += fmt.Sprintf("Retention: reports=%dd events=%dd max_report_files=%d max_closed_paper_orders=%d max_candles_per_pair=%d max_analysis_rows=%d max_plan_rows=%d wal_checkpoint=%v\n", result.Config.ReportRetentionDays, result.Config.EventRetentionDays, result.Config.MaxReportFiles, result.Config.MaxClosedPaperOrders, result.Config.MaxCandlesPerSymbolInterval, result.Config.MaxAnalysisRows, result.Config.MaxPlanRows, result.Config.WALCheckpoint)
	md += fmt.Sprintf("Deleted: reports=%d live_order_events=%d live_position_events=%d closed_paper_orders=%d candles=%d analyses=%d plans=%d report_files=%d\n", result.ReportsDeleted, result.LiveOrderEventsDeleted, result.LivePositionEventsDeleted, result.ClosedPaperOrdersDeleted, result.CandlesDeleted, result.AnalysesDeleted, result.PlansDeleted, result.ReportFilesDeleted)
	if result.WALCheckpointed {
		md += "WAL checkpoint: done\n"
	}
	md += "Live orders, live fills, live positions, and operator settings were not pruned.\n"
	return md
}

func saveJSONFile(dir, name string, v any) error {
	return reportio.WriteJSON(dir, name, v)
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func intArgValue(args []string, key string) (int, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

func floatArgValue(args []string, key string) (float64, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func sendTelegram(ctx context.Context, cfg config.Config, label, text string) {
	token := firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN"))
	chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
	result, err := telegramManager.Send(ctx, token, chatID, label, text)
	if err != nil {
		if errors.Is(err, notify.ErrTelegramSkipped) {
			log.Printf("telegram skipped [%s]: missing token/chat", label)
			return
		}
		log.Printf("telegram warning [%s]: %v", label, err)
		return
	}
	log.Printf("telegram sent ok [%s] msg_id=%d", label, result.MessageID)
}

func formatStatus(db *storage.DB) (string, error) {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return "no analysis yet; run fetch then analyze", nil
	}
	plan, _ := db.LatestPlan()
	orders, err := db.OpenPaperOrders()
	if err != nil {
		return "", err
	}
	halted, _ := db.IsHalted()
	haltStr := "INACTIVE"
	if halted {
		haltStr = "ACTIVE"
	}
	out := fmt.Sprintf(`BTC Agent Status
- Operator halt: %s
- BTC: %s | permission %s
- Trend score: %.1f
- Risk: %s | falling knife %s | FOMO %s
- Support: %.2f - %.2f
- Deep support: %.2f - %.2f
- Resistance: %.2f - %.2f
- Accumulation: %.2f - %.2f
- Invalidation: %.2f - %.2f

Flow
- Bias: %s | score %.2f
- Daily: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- 4H: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- Summary: %s

Agent 2
- State: %s
`, haltStr, analysis.MarketRegime, analysis.ActionPermission, analysis.TrendScore, analysis.RiskLevel, analysis.FallingKnifeRisk, analysis.FomoRisk, analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High, analysis.DeepSupportZone.Low, analysis.DeepSupportZone.High, analysis.ResistanceZone.Low, analysis.ResistanceZone.High, analysis.AccumulationZone.Low, analysis.AccumulationZone.High, analysis.InvalidationZone.Low, analysis.InvalidationZone.High, analysis.Flow.Bias, analysis.Flow.Score, analysis.Flow.Daily.SweepLow, analysis.Flow.Daily.ReclaimSupport, analysis.Flow.Daily.Absorption, analysis.Flow.Daily.FailedBreakout, analysis.Flow.Daily.Distribution, analysis.Flow.FourHour.SweepLow, analysis.Flow.FourHour.ReclaimSupport, analysis.Flow.FourHour.Absorption, analysis.Flow.FourHour.FailedBreakout, analysis.Flow.FourHour.Distribution, analysis.Flow.Summary, plan.State)
	if len(plan.Rotation) > 0 {
		out += "- Asset ranking:\n"
		for _, r := range plan.Rotation {
			out += fmt.Sprintf("  - #%d %s score %.2f rel %.2f%% flow %s | %s\n", r.Rank, r.Symbol, r.Score, r.RelativeReturn*100, r.FlowBias, r.Reason)
		}
	}
	if len(plan.Watchlist.Candidates) > 0 {
		out += "- Watchlist gần đạt điều kiện:\n"
		limit := len(plan.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range plan.Watchlist.Candidates[:limit] {
			out += fmt.Sprintf("  - %s readiness %.2f tier=%s actionable=%v | checklist=%s | next=%s\n", c.Symbol, c.ReadinessScore, c.Tier, c.Actionable, agent2.ChecklistSummary(c.EntryChecklist), c.NextTrigger)
		}
	}
	if len(plan.Assets) == 0 {
		out += "- Assets: chưa có kế hoạch chi tiết hoặc Agent 1 chưa ALLOWED\n"
	} else {
		for _, asset := range plan.Assets {
			out += fmt.Sprintf("- %s: %s | rank %d score %.2f | asset flow %s %.2f | RR %.2f | %s\n", asset.Symbol, asset.State, asset.RotationRank, asset.RotationScore, asset.AssetFlowBias, asset.AssetFlowScore, asset.RewardRisk, asset.Reason)
		}
	}
	out += fmt.Sprintf("- Open paper orders: %d", len(orders))
	return out, nil
}

func runReconcileLiveOrders(ctx context.Context, cfg config.Config, db *storage.DB) error {
	return runReconcileLiveOrdersWithNotify(ctx, cfg, db, true)
}

func runReconcileLiveOrdersWithNotify(ctx context.Context, cfg config.Config, db *storage.DB, notifyTelegram bool) error {
	open, err := db.OpenLiveOrders()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}

	var result liveguard.ReconcileResult
	if len(open) == 0 {
		// #11: no open orders — skip OKX client creation entirely; ReconcileOrders with
		// nil reader is safe but pointless. Build empty clean result directly.
		result = liveguard.ReconcileOrders(ctx, nil, open)
	} else {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err != nil {
			return fmt.Errorf("create okx client: %w", err)
		}
		result = liveguard.ReconcileOrders(ctx, client, open)
	}

	ledgerReport := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), ManualCheckRequired: []string{}, Events: []live.LivePositionEvent{}}
	for _, o := range result.Orders {
		if o.Status != live.StatusUnknownNeedsManualCheck {
			if err := db.SaveLiveOrderStatus(o); err != nil {
				return fmt.Errorf("save reconciled live order %s/%s: %w", o.ClientOrderID, o.OrderID, err)
			}
		}
		if err := db.SaveLiveOrderEvent(o); err != nil {
			return fmt.Errorf("save live order event %s/%s: %w", o.ClientOrderID, o.OrderID, err)
		}
		if err := applyLedgerUpdate(db, o, &ledgerReport); err != nil {
			return err
		}
	}

	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	ledgerReport.Positions = positions
	ledgerReport.Summary = liveguard.LiveLedgerSummary(ledgerReport)

	if err := saveJSONFile("reports", "live_reconcile_latest.json", result); err != nil {
		return err
	}

	md := reconcileMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_reconcile_latest.md"), []byte(md), 0600); err != nil {
		return err
	}

	if err := writeLivePositionReport(ledgerReport); err != nil {
		return err
	}

	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "reconcile-live-orders", telegramreport.ReconcileHumanText(result, ledgerReport))
	}

	fmt.Println(md)
	fmt.Println(livePositionMarkdown(ledgerReport))
	return nil
}

func applyLedgerUpdate(db *storage.DB, status live.OrderStatus, report *liveguard.LiveLedgerReport) error {
	if status.Status == live.StatusUnknownNeedsManualCheck {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s/%s status unknown", status.ClientOrderID, status.OrderID))
		return nil
	}
	if status.ClientOrderID == "" && status.OrderID == "" {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s missing order identifiers", status.InstID))
		return nil
	}
	previous, _, err := db.LiveFillSnapshot(status.ClientOrderID, status.OrderID)
	if err != nil {
		return fmt.Errorf("load live fill snapshot %s/%s: %w", status.ClientOrderID, status.OrderID, err)
	}
	event, ok, err := liveguard.BuildPositionEvent(previous, status, time.Now())
	if err != nil {
		report.ManualCheckRequired = append(report.ManualCheckRequired, err.Error())
		return nil
	}
	if !ok {
		return nil
	}
	position, err := db.ApplyLivePositionEvent(event)
	if err != nil {
		report.ManualCheckRequired = append(report.ManualCheckRequired, err.Error())
		return nil
	}
	event.PositionQty = position.Quantity
	event.AvgEntryPrice = position.AvgEntryPrice
	if err := db.SaveLivePositionEvent(event); err != nil {
		return fmt.Errorf("save live position event %s/%s: %w", event.ClientOrderID, event.OrderID, err)
	}
	snapshot := liveguard.FillSnapshotFromStatus(status)
	if snapshot.ClientOrderID == "" {
		snapshot.ClientOrderID = previous.ClientOrderID
	}
	if snapshot.ClientOrderID == "" {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s/%s missing client_order_id for fill snapshot", status.InstID, status.OrderID))
		return nil
	}
	if err := db.SaveLiveFillSnapshot(snapshot); err != nil {
		return fmt.Errorf("save live fill snapshot %s/%s: %w", snapshot.ClientOrderID, snapshot.OrderID, err)
	}
	report.Events = append(report.Events, event)
	report.Updated++
	return nil
}

func runLivePositions(cfg config.Config, db *storage.DB) error {
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	report := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), Positions: positions, Events: []live.LivePositionEvent{}, ManualCheckRequired: []string{}}
	report.Summary = liveguard.LiveLedgerSummary(report)
	if err := writeLivePositionReport(report); err != nil {
		return err
	}
	md := livePositionMarkdown(report)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(context.Background(), cfg, "live-positions", telegramreport.PositionHumanText(report))
	}
	fmt.Println(md)
	return nil
}

func writeLivePositionReport(report liveguard.LiveLedgerReport) error {
	if err := saveJSONFile("reports", "live_position_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "live_position_latest.md"), []byte(livePositionMarkdown(report)), 0600)
}

func reconcileMarkdown(result liveguard.ReconcileResult) string {
	md := fmt.Sprintf("LIVE RECONCILIATION REPORT\n\nGenerated: %s\nSummary: %s\nChecked: %d | Updated: %d | Unknown: %d\nSafety: %s | %s\n\n",
		result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Checked, result.Updated, result.Unknown, result.Safety.Status, result.Safety.Summary)
	md += "No order was placed.\n"
	if len(result.Orders) > 0 {
		md += "\nOrders:\n"
		for _, o := range result.Orders {
			md += fmt.Sprintf("- %s: clOrdId=%s ordId=%s status=%s px=%.2f qty=%.4f avgPx=%.2f\n",
				o.InstID, o.ClientOrderID, o.OrderID, o.Status, o.Price, o.Quantity, o.AvgPrice)
		}
	}
	return md
}

func livePositionMarkdown(result liveguard.LiveLedgerReport) string {
	md := fmt.Sprintf("LIVE POSITION LEDGER\n\nGenerated: %s\nSummary: %s\nLedger updates: %d | Manual checks: %d\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Updated, len(result.ManualCheckRequired))
	md += "No order was placed.\n"
	if len(result.Positions) == 0 {
		md += "No live positions recorded.\n"
	} else {
		md += "\nPositions:\n"
		for _, p := range result.Positions {
			md += fmt.Sprintf("- %s: qty=%.8f avg_entry=%.8f cost=%.2f fee_total=%.8f fee_ccy=%s\n", p.Symbol, p.Quantity, p.AvgEntryPrice, p.CostBasis, p.FeeTotal, p.FeeCurrency)
		}
	}
	if len(result.Events) > 0 {
		md += "\nNew ledger events:\n"
		for _, e := range result.Events {
			md += fmt.Sprintf("- %s: order=%s delta_qty=%.8f fill_price=%.8f fee_delta=%.8f status=%s\n", e.Symbol, firstNonEmpty(e.ClientOrderID, e.OrderID), e.DeltaQuantity, e.FillPrice, e.FeeDelta, e.Status)
		}
	}
	if len(result.ManualCheckRequired) > 0 {
		md += "\nManual check required:\n"
		for _, item := range result.ManualCheckRequired {
			md += "- " + item + "\n"
		}
	}
	return md
}

func runSimulateLiveManager(cfg config.Config) error {
	result := liveguard.RunLiveManagerSimulation(cfg)
	if err := saveJSONFile("reports", "live_manager_simulation_latest.json", result); err != nil {
		return err
	}
	md := liveManagerSimulationMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_manager_simulation_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	if !result.Passed {
		return fmt.Errorf("live manager simulation failed")
	}
	return nil
}

func liveManagerSimulationMarkdown(result liveguard.LiveManagerSimulationResult) string {
	md := fmt.Sprintf("LIVE MANAGER SIMULATION\n\nGenerated: %s\nSummary: %s\nPassed: %v\nScenarios: %d\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Passed, len(result.Scenarios))
	for i, scenario := range result.Scenarios {
		status := "PASS"
		if !scenario.Passed {
			status = "FAIL"
		}
		md += fmt.Sprintf("%d) %s — %s\n", i+1, scenario.Name, status)
		md += fmt.Sprintf("   Expected: %s\n", scenario.Expected)
		md += fmt.Sprintf("   Result: desired=%d kept=%d canceled=%d replaced=%d placed=%d blocked=%d\n", len(scenario.Result.Desired), len(scenario.Result.Kept), len(scenario.Result.Canceled), len(scenario.Result.Replaced), len(scenario.Result.Placed), len(scenario.Result.Blocked))
		if scenario.Failure != "" {
			md += "   Failure: " + scenario.Failure + "\n"
		}
	}
	md += "\nNo real order was placed or canceled. Simulation only.\n"
	return md
}

func runCancelAllLiveOrders(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool) error {
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	result := liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleCompleted, PlanState: agent2.StateNoTrade, DryRun: dryRun}
	if len(open) == 0 {
		result.Summary = "cancel-all: no open live orders"
	} else if dryRun {
		for _, order := range open {
			result.Canceled = append(result.Canceled, liveguard.ManagedOrderDecision{Action: "would_cancel", Symbol: live.InternalSymbol(order.InstID), LayerIndex: order.LayerIndex, Order: order, Reason: "emergency cancel all dry-run"})
		}
		result.Status = liveguard.ManagedCycleDryRun
		result.Summary = fmt.Sprintf("cancel-all dry-run: would cancel %d open live orders", len(result.Canceled))
	} else {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err != nil {
			return fmt.Errorf("create okx client: %w", err)
		}
		for _, order := range open {
			decision := liveguard.ManagedOrderDecision{Action: "cancel", Symbol: live.InternalSymbol(order.InstID), LayerIndex: order.LayerIndex, Order: order, Reason: "emergency cancel all"}
			cancel, err := client.CancelOrder(ctx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
			decision.CancelResult = cancel
			if err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = liveguard.ManagedCyclePartial
				continue
			}
			result.Canceled = append(result.Canceled, decision)
			status := order
			status.Status = live.StatusCanceled
			status.UpdatedAt = time.Now().Unix()
			status.LastManagementAction = "emergency cancel all"
			if err := db.SaveLiveOrderStatus(status); err != nil {
				return fmt.Errorf("save canceled order: %w", err)
			}
			if err := db.SaveLiveOrderEvent(status); err != nil {
				return fmt.Errorf("save canceled order event: %w", err)
			}
		}
		if result.Status == "" || result.Status == liveguard.ManagedCycleCompleted {
			result.Status = liveguard.ManagedCycleCompleted
		}
		result.Summary = fmt.Sprintf("cancel-all: canceled=%d blocked=%d", len(result.Canceled), len(result.Blocked))
		if len(result.Canceled) > 0 {
			if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
				log.Printf("post-cancel-all reconcile warning: %v", err)
			}
		}
	}
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("cancel-all: canceled=%d blocked=%d", len(result.Canceled), len(result.Blocked))
	}
	if err := saveJSONFile("reports", "cancel_all_live_orders_latest.json", result); err != nil {
		return err
	}
	md := autoLiveManagementMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "cancel_all_live_orders_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "cancel-all-live-orders", telegramreport.LiveOrderManagementHumanText(result))
	}
	fmt.Println(md)
	return nil
}

func runOperatorHalt(db *storage.DB) error {
	if err := db.SetHaltStatus(true); err != nil {
		return fmt.Errorf("set halt status: %w", err)
	}
	fmt.Println("Operator halt: ACTIVE (Live trading halted)")
	return nil
}

func runOperatorResume(db *storage.DB) error {
	if err := db.SetHaltStatus(false); err != nil {
		return fmt.Errorf("clear halt status: %w", err)
	}
	fmt.Println("Operator halt: INACTIVE (Live trading resumed)")
	return nil
}

func runOperatorStatus(db *storage.DB) error {
	halted, err := db.IsHalted()
	if err != nil {
		return fmt.Errorf("read halt status: %w", err)
	}
	status := "INACTIVE"
	if halted {
		status = "ACTIVE (trading halted)"
	}
	fmt.Printf("Operator halt: %s\n", status)
	return nil
}
