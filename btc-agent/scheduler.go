package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

func runScheduler(ctx context.Context, cfg config.Config, db *storage.DB, runNow bool) error {
	tz := cfg.App.Timezone
	if tz == "" {
		tz = "Asia/Ho_Chi_Minh"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", tz, err)
	}

	log.Printf("[Scheduler] Started. Timezone: %s", tz)

	dailyTime := cfg.App.DailyRunTime
	if dailyTime == "" {
		dailyTime = "08:00"
	}
	log.Printf("[Scheduler] Daily run scheduled at: %s", dailyTime)

	reconcileInterval := 15 * time.Minute
	if cfg.App.ReconcileIntervalMinutes > 0 {
		reconcileInterval = time.Duration(cfg.App.ReconcileIntervalMinutes) * time.Minute
	}
	log.Printf("[Scheduler] Live order reconciliation interval: %v (Live enabled: %v)", reconcileInterval, cfg.Live.Enabled)

	// Calculate initial next daily run time
	nextDaily, err := getNextRunTime(dailyTime, loc, time.Now().In(loc))
	if err != nil {
		return err
	}
	log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))

	// Setup OS signal handling for graceful shutdown
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("[Scheduler] Received signal %v, shutting down scheduler gracefully...", sig)
		cancel()
	}()

	// Execute runDaily immediately if requested
	if runNow {
		log.Println("[Scheduler] Executing initial daily run (--run-now)...")
		if err := runDaily(shutdownCtx, cfg, db); err != nil {
			log.Printf("[Scheduler] Initial daily run error: %v", err)
		}
	}

	// Track last reconciliation execution time
	// Run reconciliation once on start if live is enabled
	var lastReconcile time.Time
	if cfg.Live.Enabled {
		log.Println("[Scheduler] Executing initial live order reconciliation...")
		if err := runReconcileLiveOrders(shutdownCtx, cfg, db); err != nil {
			log.Printf("[Scheduler] Initial reconciliation error: %v", err)
		}
		lastReconcile = time.Now()
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-shutdownCtx.Done():
			log.Println("[Scheduler] Stopped.")
			return nil
		case <-ticker.C:
			now := time.Now().In(loc)

			// Check if it's time for daily run
			if now.After(nextDaily) {
				log.Printf("[Scheduler] Triggering scheduled daily run at %s...", now.Format("15:04:05 MST"))
				if err := runDaily(shutdownCtx, cfg, db); err != nil {
					log.Printf("[Scheduler] Daily run error: %v", err)
				}
				// Calculate next daily run time
				nextDaily, err = getNextRunTime(dailyTime, loc, now)
				if err != nil {
					log.Printf("[Scheduler] Error calculating next run time: %v", err)
					// Fallback to 24 hours from now
					nextDaily = now.Add(24 * time.Hour)
				}
				log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))
			}

			// Check if it's time for live order reconciliation
			if cfg.Live.Enabled && time.Since(lastReconcile) >= reconcileInterval {
				log.Println("[Scheduler] Triggering scheduled live order reconciliation...")
				if err := runReconcileLiveOrders(shutdownCtx, cfg, db); err != nil {
					log.Printf("[Scheduler] Reconciliation error: %v", err)
				}
				lastReconcile = time.Now()
			}
		}
	}
}

func getNextRunTime(dailyRunTime string, loc *time.Location, now time.Time) (time.Time, error) {
	var hour, min int
	if _, err := fmt.Sscanf(dailyRunTime, "%d:%d", &hour, &min); err != nil {
		return time.Time{}, fmt.Errorf("parse daily_run_time %q: %w", dailyRunTime, err)
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
	if t.Before(now) || t.Equal(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}
