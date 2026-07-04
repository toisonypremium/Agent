package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/aiagent"
	"btc-agent/internal/aieval"
	"btc-agent/internal/backtest"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/market"
	"btc-agent/internal/notify"
	"btc-agent/internal/storage"
)

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
	case "export-training":
		return runExportTraining(cfg, db)
	case "run-ai-watch":
		return runAIWatch(ctx, cfg, db)
	case "live-proof":
		return runLiveProof(ctx, cfg, db)
	case "execute-live-proof-order":
		return runExecuteLiveProofOrder(ctx, cfg, db, argValue(args, "--confirm"))
	case "reconcile-live-orders":
		return runReconcileLiveOrders(ctx, cfg, db)
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: btc-agent <fetch|analyze|plan|run-daily|run-ai-watch|backtest|export-training|eval-ai|live-proof|execute-live-proof-order|reconcile-live-orders|status> --config config.yaml")
}

func fetch(ctx context.Context, cfg config.Config, db *storage.DB) error {
	client := exchange.NewBinance(cfg.Data.BinanceBaseURL)
	symbols := append([]string{cfg.Data.Symbols.BTC}, cfg.Data.Symbols.Assets...)
	for _, sym := range symbols {
		for _, interval := range cfg.Data.Intervals {
			candles, err := client.Klines(ctx, sym, interval, cfg.Data.CandleLimit)
			if err != nil {
				return fmt.Errorf("fetch %s %s: %w", sym, interval, err)
			}
			if err := db.SaveCandles(candles); err != nil {
				return err
			}
			fmt.Printf("saved %d candles %s %s\n", len(candles), sym, interval)
		}
	}
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
	if cfg.Notify.Enabled {
		switch cfg.Notify.Provider {
		case "telegram":
			token := firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN"))
			chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
			if err := notify.Telegram(ctx, token, chatID, report); err != nil {
				log.Printf("telegram warning: %v", err)
			}
		case "ntfy":
			if err := notify.Ntfy(ctx, cfg.Notify.NtfyTopic, report); err != nil {
				log.Printf("ntfy warning: %v", err)
			}
		}
	}
	fmt.Println(report)
	return nil
}

func runBacktest(cfg config.Config, db *storage.DB) error {
	daily, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return err
	}
	result, err := backtest.RunBTC(backtest.Config{MinWindow1D: 60, HorizonDays: []int{1, 3, 7, 14}}, daily)
	if err != nil {
		return err
	}
	flowAudit, err := backtest.RunBTCFlowBottleneckAudit(map[string][]market.Candle{"1d": daily}, backtest.BTCFlowBottleneckAuditConfig{})
	if err != nil {
		result.BTCFlowBottleneckAudit = backtest.BTCFlowBottleneckAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.BTCFlowBottleneckAudit = flowAudit
	}
	qualityAudit, err := backtest.RunFlowParamQualityAudit(map[string][]market.Candle{"1d": daily}, backtest.FlowParamQualityAuditConfig{})
	if err != nil {
		result.FlowParamQualityAudit = backtest.FlowParamQualityAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.FlowParamQualityAudit = qualityAudit
	}
	permissionAudit, err := backtest.RunBTCPermissionAudit(cfg, map[string][]market.Candle{"1d": daily}, backtest.BTCPermissionAuditConfig{})
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
	sim, err := backtest.RunAgent2Simulation(cfg, map[string][]market.Candle{"1d": daily}, assets)
	if err != nil {
		result.Agent2Simulation = backtest.Agent2Simulation{Enabled: false, Assets: map[string]backtest.AssetSimStats{}, Summary: err.Error()}
	} else {
		result.Agent2Simulation = sim
	}
	watchAudit, err := backtest.RunWatchlistTriggerAudit(cfg, map[string][]market.Candle{"1d": daily}, assets, backtest.WatchlistTriggerAuditConfig{})
	if err != nil {
		result.WatchlistTriggerAudit = backtest.WatchlistTriggerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.WatchlistTriggerAudit = watchAudit
	}
	checklistAudit, err := backtest.RunChecklistPassCountAudit(cfg, map[string][]market.Candle{"1d": daily}, assets, backtest.ChecklistPassCountAuditConfig{})
	if err != nil {
		result.ChecklistPassCountAudit = backtest.ChecklistPassCountAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.ChecklistPassCountAudit = checklistAudit
	}
	audit, err := backtest.RunLayerAudit(cfg, map[string][]market.Candle{"1d": daily}, assets, backtest.LayerAuditConfig{})
	if err != nil {
		result.LayerAudit = backtest.LayerAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.LayerAudit = audit
	}
	exitAudit, err := backtest.RunExitAudit(cfg, map[string][]market.Candle{"1d": daily}, assets, backtest.ExitAuditConfig{})
	if err != nil {
		result.ExitAudit = backtest.ExitAuditResult{Enabled: false, Summary: err.Error()}
	} else {
		result.ExitAudit = exitAudit
	}
	md := backtest.Markdown(result)
	if err := backtest.SaveReports("reports", result, md); err != nil {
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
		token := firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN"))
		chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
		if err := notify.Telegram(ctx, token, chatID, report.TelegramText); err != nil {
			log.Printf("telegram warning: %v", err)
		}
	}
	fmt.Println(report.TelegramText)
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
	fmt.Println(md)
	return nil
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
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		_ = notify.Telegram(ctx, firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN")), firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID")), liveOrderAttemptText(proof))
	}
	result := liveguard.ExecuteManualProofOrder(ctx, cfg, proof, confirm, placer)
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
		_ = notify.Telegram(ctx, firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN")), firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID")), md)
	}
	fmt.Println(md)
	return nil
}

func liveOrderAttemptText(proof liveguard.Proof) string {
	return fmt.Sprintf("MANUAL LIVE ORDER ATTEMPT\nproof=%s symbol=%s inst_id=%s notional=%.2f\nNo order yet; hard gates still apply.", proof.Status, proof.Candidate.Symbol, proof.Preflight.InstID, proof.Candidate.Notional)
}

func liveOrderMarkdown(result liveguard.ExecutionResult) string {
	md := fmt.Sprintf("MANUAL LIVE PROOF ORDER\n\nStatus: %s\nSummary: %s\nProof status: %s\n", result.Status, result.Summary, result.ProofStatus)
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

func saveJSONFile(dir, name string, v any) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), b, 0600)
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
	out := fmt.Sprintf(`BTC Agent Status
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
`, analysis.MarketRegime, analysis.ActionPermission, analysis.TrendScore, analysis.RiskLevel, analysis.FallingKnifeRisk, analysis.FomoRisk, analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High, analysis.DeepSupportZone.Low, analysis.DeepSupportZone.High, analysis.ResistanceZone.Low, analysis.ResistanceZone.High, analysis.AccumulationZone.Low, analysis.AccumulationZone.High, analysis.InvalidationZone.Low, analysis.InvalidationZone.High, analysis.Flow.Bias, analysis.Flow.Score, analysis.Flow.Daily.SweepLow, analysis.Flow.Daily.ReclaimSupport, analysis.Flow.Daily.Absorption, analysis.Flow.Daily.FailedBreakout, analysis.Flow.Daily.Distribution, analysis.Flow.FourHour.SweepLow, analysis.Flow.FourHour.ReclaimSupport, analysis.Flow.FourHour.Absorption, analysis.Flow.FourHour.FailedBreakout, analysis.Flow.FourHour.Distribution, analysis.Flow.Summary, plan.State)
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
	open, err := db.OpenLiveOrders()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}

	var result liveguard.ReconcileResult
	if len(open) == 0 {
		result = liveguard.ReconcileOrders(ctx, nil, open)
	} else {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err != nil {
			return fmt.Errorf("create okx client: %w", err)
		}
		result = liveguard.ReconcileOrders(ctx, client, open)
	}

	for _, o := range result.Orders {
		if o.Status != live.StatusUnknownNeedsManualCheck {
			if err := db.SaveLiveOrderStatus(o); err != nil {
				return fmt.Errorf("save reconciled live order %s/%s: %w", o.ClientOrderID, o.OrderID, err)
			}
		}
		if err := db.SaveLiveOrderEvent(o); err != nil {
			return fmt.Errorf("save live order event %s/%s: %w", o.ClientOrderID, o.OrderID, err)
		}
	}

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

	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		_ = notify.Telegram(ctx, firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN")), firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID")), md)
	}

	fmt.Println(md)
	return nil
}

func reconcileMarkdown(result liveguard.ReconcileResult) string {
	md := fmt.Sprintf("LIVE RECONCILIATION REPORT\n\nGenerated: %s\nSummary: %s\nChecked: %d | Updated: %d | Unknown: %d\n\n",
		result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Checked, result.Updated, result.Unknown)
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
