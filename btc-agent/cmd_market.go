package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
	"btc-agent/internal/notify"
	"btc-agent/internal/paper"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
)

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
	analysis = applyMicrostructurePermissionGate(cfg, analysis, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
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
	p = applyMicrostructureAssetGate(cfg, p, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
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

func monitorPlan(cfg config.Config, db *storage.DB) (agent2.Plan, error) {
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
	p = applyMicrostructureAssetGate(cfg, p, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
	if err := db.SavePlan(p); err != nil {
		return p, err
	}
	orders := agent2.OrdersFromPlan(p, cfg.Execution.OrderExpiryHours)
	if err := db.SaveOrders(orders); err != nil {
		return p, err
	}
	return p, nil
}

func runPaperManager(cfg config.Config, db *storage.DB) error {
	orders, err := db.OpenPaperOrders()
	if err != nil {
		return fmt.Errorf("load open paper orders: %w", err)
	}
	plan, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	candlesBySymbol := map[string][]market.Candle{}
	seen := map[string]bool{}
	for _, order := range orders {
		symbol := strings.ToUpper(order.Symbol)
		if symbol == "" || seen[symbol] {
			continue
		}
		seen[symbol] = true
		candles, err := db.LoadCandles(symbol, "1d", cfg.Data.CandleLimit)
		if err != nil {
			return fmt.Errorf("load paper candles %s: %w", symbol, err)
		}
		candlesBySymbol[symbol] = candles
	}
	result := paper.ManageOpenOrders(time.Now(), orders, candlesBySymbol, plan)
	for _, event := range result.Events {
		if err := db.UpdatePaperOrderStatus(event.OrderID, event.NewStatus, event.Reason); err != nil {
			return fmt.Errorf("update paper order %s: %w", event.OrderID, err)
		}
	}
	if err := saveJSONFile("reports", "paper_manager_latest.json", result); err != nil {
		return err
	}
	md := paper.Markdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "paper_manager_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
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
			log.Println("Active limit state reached in daily run (supervisor disabled). Executing automatic live order placement...")
			if err := runAutoLiveOrder(ctx, cfg, db, false); err != nil {
				log.Printf("Automatic live order placement error: %v", err)
			}
		}
	}
	if notifyTelegram && cfg.Notify.Enabled {
		switch cfg.Notify.Provider {
		case "telegram":
			sendScheduledTelegram(ctx, cfg, "run-daily", telegramreport.DailyHumanText(analysis, p))
		case "ntfy":
			if err := notify.Ntfy(ctx, cfg.Notify.NtfyTopic, report); err != nil {
				log.Printf("ntfy warning: %v", err)
			}
		}
	}
	fmt.Println(report)
	return nil
}
