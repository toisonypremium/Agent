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

	maintenanceEnabled := cfg.Maintenance.Enabled && cfg.Maintenance.SchedulerEnabled
	maintenanceTime := cfg.Maintenance.SchedulerTime
	if maintenanceTime == "" {
		maintenanceTime = "03:30"
	}
	log.Printf("[Scheduler] Maintenance schedule: %s (enabled: %v)", maintenanceTime, maintenanceEnabled)

	// Calculate initial next daily run time
	nextDaily, err := getNextRunTime(dailyTime, loc, time.Now().In(loc))
	if err != nil {
		return err
	}
	log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))

	var nextMaintenance time.Time
	if maintenanceEnabled {
		nextMaintenance, err = getNextRunTime(maintenanceTime, loc, time.Now().In(loc))
		if err != nil {
			return err
		}
		log.Printf("[Scheduler] Next scheduled maintenance run: %s", nextMaintenance.Format("2006-01-02 15:04:05 MST"))
	}

	// Setup OS signal handling for graceful shutdown
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	go func() {
		select {
		case sig := <-sigChan:
			log.Printf("[Scheduler] Received signal %v, shutting down scheduler gracefully...", sig)
			cancel()
		case <-shutdownCtx.Done():
		}
	}()

	// Execute runDaily immediately if requested
	if runNow {
		log.Println("[Scheduler] Executing initial daily run (--run-now)...")
		if err := runDaily(shutdownCtx, cfg, db); err != nil {
			log.Printf("[Scheduler] Initial daily run error: %v", err)
		}
	}

	// Run reconciliation once on start if live is enabled, then schedule future ticks.
	var nextReconcile time.Time
	if cfg.Live.Enabled {
		log.Println("[Scheduler] Executing initial live order reconciliation...")
		if err := runReconcileLiveOrders(shutdownCtx, cfg, db); err != nil {
			log.Printf("[Scheduler] Initial reconciliation error: %v", err)
		}
		nextReconcile = time.Now().Add(reconcileInterval)
	}

	for {
		waitUntil := nextDaily
		if cfg.Live.Enabled && nextReconcile.Before(waitUntil) {
			waitUntil = nextReconcile
		}
		if maintenanceEnabled && nextMaintenance.Before(waitUntil) {
			waitUntil = nextMaintenance
		}
		wait := time.Until(waitUntil)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-shutdownCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			log.Println("[Scheduler] Stopped.")
			return nil
		case <-timer.C:
		}

		now := time.Now().In(loc)
		if !now.Before(nextDaily) {
			log.Printf("[Scheduler] Triggering scheduled daily run at %s...", now.Format("15:04:05 MST"))
			if err := runDaily(shutdownCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Daily run error: %v", err)
			}
			now = time.Now().In(loc)
			nextDaily, err = getNextRunTime(dailyTime, loc, now)
			if err != nil {
				log.Printf("[Scheduler] Error calculating next run time: %v", err)
				nextDaily = now.Add(24 * time.Hour)
			}
			log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))
		}

		if cfg.Live.Enabled && !time.Now().Before(nextReconcile) {
			log.Println("[Scheduler] Triggering scheduled live order reconciliation...")
			if err := runReconcileLiveOrders(shutdownCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Reconciliation error: %v", err)
			}
			nextReconcile = time.Now().Add(reconcileInterval)
		}

		if maintenanceEnabled && !time.Now().Before(nextMaintenance) {
			log.Println("[Scheduler] Triggering scheduled maintenance...")
			if err := runMaintenance(cfg, db); err != nil {
				log.Printf("[Scheduler] Maintenance error: %v", err)
			}
			now = time.Now().In(loc)
			nextMaintenance, err = getNextRunTime(maintenanceTime, loc, now)
			if err != nil {
				log.Printf("[Scheduler] Error calculating next maintenance run time: %v", err)
				nextMaintenance = now.Add(24 * time.Hour)
			}
			log.Printf("[Scheduler] Next scheduled maintenance run: %s", nextMaintenance.Format("2006-01-02 15:04:05 MST"))
		}
	}
}

func parseClockTime(value string) (int, int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, 0, fmt.Errorf("invalid clock time %q", value)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return 0, 0, fmt.Errorf("invalid clock time %q", value)
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	min := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || min > 59 {
		return 0, 0, fmt.Errorf("invalid clock time %q", value)
	}
	return hour, min, nil
}

func getNextRunTime(dailyRunTime string, loc *time.Location, now time.Time) (time.Time, error) {
	hour, min, err := parseClockTime(dailyRunTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse daily_run_time %q: %w", dailyRunTime, err)
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
	if t.Before(now) || t.Equal(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}
