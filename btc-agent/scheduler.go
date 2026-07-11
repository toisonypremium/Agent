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
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

const (
	schedulerResearchTimeout   = 90 * time.Second
	schedulerAIWatchTimeout    = 120 * time.Second
	schedulerAITelegramTimeout = 90 * time.Second
)

func runScheduler(ctx context.Context, cfg config.Config, db *storage.DB, runNow bool, dryRun bool) error {
	if os.Getenv("BTC_AGENT_SCHEDULER_LOCK_HELD") != "true" {
		releaseLock, err := acquireSchedulerProcessLock()
		if err != nil {
			return err
		}
		defer releaseLock()
	}

	tz := cfg.App.Timezone
	if tz == "" {
		tz = "Asia/Ho_Chi_Minh"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", tz, err)
	}

	log.Printf("[Scheduler] Started. Timezone: %s", tz)
	if dryRun {
		log.Println("[Scheduler] Dry-run mode enabled: supervisor/order management cycles will not place or cancel real orders.")
	}

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

	managementInterval := 15 * time.Minute
	if cfg.Live.ManagementIntervalMinutes > 0 {
		managementInterval = time.Duration(cfg.Live.ManagementIntervalMinutes) * time.Minute
	}
	log.Printf("[Scheduler] Live supervisor interval: %v (enabled: %v)", managementInterval, cfg.Live.SupervisorEnabled)

	maintenanceEnabled := cfg.Maintenance.Enabled && cfg.Maintenance.SchedulerEnabled
	maintenanceTime := cfg.Maintenance.SchedulerTime
	if maintenanceTime == "" {
		maintenanceTime = "03:30"
	}
	log.Printf("[Scheduler] Maintenance schedule: %s (enabled: %v)", maintenanceTime, maintenanceEnabled)

	// #8: interval=0 means "run on --run-now only, no scheduled repeats".
	// Only set a positive interval when BriefIntervalMinutes > 0.
	researchInterval := time.Duration(cfg.Research.BriefIntervalMinutes) * time.Minute
	researchScheduled := cfg.Research.Enabled && researchInterval > 0
	log.Printf("[Scheduler] Research brief interval: %v scheduled=%v (enabled: %v)", researchInterval, researchScheduled, cfg.Research.Enabled)

	marketScanInterval := time.Duration(cfg.Monitoring.MarketScanIntervalMinutes) * time.Minute
	if marketScanInterval <= 0 {
		marketScanInterval = 15 * time.Minute
	}
	monitoringEnabled := cfg.Monitoring.Enabled
	log.Printf("[Scheduler] Market watch interval: %v (enabled: %v)", marketScanInterval, monitoringEnabled)

	var nextDaily time.Time
	var nextMaintenance time.Time
	var nextResearch time.Time
	var nextMarketWatch time.Time
	var nextReconcile time.Time
	var nextSupervisor time.Time
	var nextAlivePing time.Time
	var nextTelegramCommands time.Time
	var latestDoctor *liveguard.RuntimeDoctorResult
	consecutiveDoctorBlocks := 0
	consecutiveMarketErrors := 0
	var lastMarketErrorAlert time.Time

	telegramCommandsInterval := 30 * time.Second
	telegramCommandsEnabled := cfg.Notify.Enabled && cfg.Notify.Provider == "telegram"
	if telegramCommandsEnabled {
		nextTelegramCommands = time.Now().Add(telegramCommandsInterval)
		log.Printf("[Scheduler] Telegram command polling interval: %v next=%s", telegramCommandsInterval, nextTelegramCommands.Format("2006-01-02 15:04:05 MST"))
	}

	alivePingInterval := time.Duration(cfg.Live.HeartbeatIntervalMinutes) * time.Minute
	alivePingEnabled := cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && alivePingInterval > 0
	if alivePingEnabled {
		nextAlivePing = time.Now().Add(alivePingInterval)
		log.Printf("[Scheduler] Alive ping interval: %v next=%s", alivePingInterval, nextAlivePing.Format("2006-01-02 15:04:05 MST"))
	}

	heartbeat := SchedulerHeartbeat{
		PID:                   os.Getpid(),
		Status:                "starting",
		Timezone:              tz,
		Mode:                  os.Getenv("BTC_AGENT_MODE"),
		DryRun:                dryRun,
		LiveEnabled:           cfg.Live.Enabled,
		LiveSupervisorEnabled: cfg.Live.SupervisorEnabled,
		ResearchEnabled:       cfg.Research.Enabled,
		MaintenanceEnabled:    maintenanceEnabled,
		LastEvent:             "scheduler starting",
		LastEventAt:           time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeSchedulerHeartbeat(heartbeat); err != nil {
		log.Printf("[Scheduler] Heartbeat write warning: %v", err)
	}
	writeHeartbeat := func(event string) {
		heartbeat.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		heartbeat.Status = "running"
		heartbeat.LastEvent = event
		heartbeat.LastEventAt = heartbeat.GeneratedAt
		heartbeat.NextDailyRun = schedulerHeartbeatTime(nextDaily)
		heartbeat.NextMaintenanceRun = schedulerHeartbeatTime(nextMaintenance)
		heartbeat.NextResearchBrief = schedulerHeartbeatTime(nextResearch)
		heartbeat.NextMarketWatch = schedulerHeartbeatTime(nextMarketWatch)
		heartbeat.NextReconcile = schedulerHeartbeatTime(nextReconcile)
		heartbeat.NextLiveSupervisorCycle = schedulerHeartbeatTime(nextSupervisor)
		if latestDoctor != nil {
			heartbeat.DoctorStatus = string(latestDoctor.Status)
			heartbeat.DoctorSummary = latestDoctor.Summary
		}
		heartbeat.ConsecutiveDoctorBlocks = consecutiveDoctorBlocks
		heartbeat.ConsecutiveMarketErrors = consecutiveMarketErrors
		if err := writeSchedulerHeartbeat(heartbeat); err != nil {
			log.Printf("[Scheduler] Heartbeat write warning: %v", err)
		}
	}

	// Calculate initial next daily run time
	nextDaily, err = getNextRunTime(dailyTime, loc, time.Now().In(loc))
	if err != nil {
		return err
	}
	log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))

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

	runNowNotes := []string{}
	runNowResearchSummary := "research skipped"
	runNowDailyOK := false
	runNowReconcileOK := false
	var runNowSupervisor liveguard.SupervisorResult
	runNowSupervisorSet := false

	// #4: runNow sequence — research (read-only) BEFORE daily run, BEFORE supervisor.
	// For --run-now, suppress individual Telegram sends and send one combined summary.
	if runNow && cfg.Research.Enabled {
		log.Println("[Scheduler] Executing initial research doctor/brief (--run-now)...")
		researchCtx, cancel := context.WithTimeout(shutdownCtx, schedulerResearchTimeout)
		if _, err := runResearchDoctor(researchCtx, cfg); err != nil {
			log.Printf("[Scheduler] Initial research doctor error: %v", err)
			runNowNotes = append(runNowNotes, "research doctor: "+err.Error())
		}
		if brief, err := runResearchBrief(researchCtx, cfg, false); err != nil {
			log.Printf("[Scheduler] Initial research brief error: %v", err)
			runNowNotes = append(runNowNotes, "research brief: "+err.Error())
		} else {
			runNowResearchSummary = brief.Summary
		}
		cancel()
	}

	if runNow {
		log.Println("[Scheduler] Executing initial daily run (--run-now)...")
		if err := runDailyWithNotify(shutdownCtx, cfg, db, false); err != nil {
			log.Printf("[Scheduler] Initial daily run error: %v", err)
			runNowNotes = append(runNowNotes, "daily: "+err.Error())
		} else {
			runNowDailyOK = true
		}
		// AI watch runs after daily so it has fresh analysis/plan.
		if cfg.AI.Enabled {
			log.Println("[Scheduler] Executing initial AI watch (--run-now)...")
			aiCtx, cancel := context.WithTimeout(shutdownCtx, schedulerAIWatchTimeout)
			if err := runAIWatch(aiCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Initial AI watch error: %v", err)
				runNowNotes = append(runNowNotes, "ai-watch: "+err.Error())
			}
			cancel()
		}
	}

	if researchScheduled {
		nextResearch = time.Now().Add(researchInterval)
		log.Printf("[Scheduler] Next research brief: %s", nextResearch.Format("2006-01-02 15:04:05 MST"))
	} else if cfg.Research.Enabled {
		// #8: interval=0 → run on --run-now only; no scheduled repeats.
		log.Printf("[Scheduler] Research enabled but brief_interval_minutes=0: no scheduled repeats.")
	}

	if monitoringEnabled {
		if !runNow {
			log.Println("[Scheduler] Executing initial market watch...")
			if report, err := runMarketWatch(shutdownCtx, cfg, db, true); err != nil {
				consecutiveMarketErrors++
				log.Printf("[Scheduler] Initial market watch error: %v", err)
				nextMarketWatch = time.Now().Add(marketWatchFailureDelay(marketScanInterval, consecutiveMarketErrors))
			} else {
				consecutiveMarketErrors = 0
				nextMarketWatch = time.Now().Add(marketWatchSuccessDelay(marketScanInterval, report))
			}
		} else {
			nextMarketWatch = time.Now().Add(marketScanInterval)
		}
		log.Printf("[Scheduler] Next market watch: %s", nextMarketWatch.Format("2006-01-02 15:04:05 MST"))
	}

	// Run reconciliation once on start if live is enabled, then schedule future ticks.
	if cfg.Live.Enabled {
		log.Println("[Scheduler] Executing initial live order reconciliation...")
		notifyReconcile := !runNow
		if err := runReconcileLiveOrdersWithNotify(shutdownCtx, cfg, db, notifyReconcile); err != nil {
			log.Printf("[Scheduler] Initial reconciliation error: %v", err)
			if runNow {
				runNowNotes = append(runNowNotes, "reconcile: "+err.Error())
			}
		} else if runNow {
			runNowReconcileOK = true
		}
		nextReconcile = time.Now().Add(reconcileInterval)
	}

	liveSupervisor := liveSupervisorState{}
	if cfg.Live.SupervisorEnabled {
		doctor, err := runLiveDoctor(shutdownCtx, cfg, db)
		if err != nil {
			log.Printf("[Scheduler] Live doctor error: %v", err)
		} else {
			latestDoctor = &doctor
			log.Printf("[Scheduler] Live doctor status: %s", doctor.Summary)
			if !dryRun && doctor.Status == liveguard.DoctorBlock {
				log.Printf("[Scheduler] Live supervisor real management blocked by doctor: %s", doctor.Summary)
			}
		}
		nextSupervisor = time.Now().Add(managementInterval)
		log.Printf("[Scheduler] Next live supervisor cycle: %s", nextSupervisor.Format("2006-01-02 15:04:05 MST"))
		if runNow {
			log.Println("[Scheduler] Executing initial live supervisor cycle (--run-now)...")
			if supervisor, err := runLiveSupervisorCycleWithDoctorNotify(shutdownCtx, cfg, db, &liveSupervisor, dryRun, latestDoctor, false); err != nil {
				log.Printf("[Scheduler] Initial live supervisor error: %v", err)
				runNowNotes = append(runNowNotes, "supervisor: "+err.Error())
			} else {
				runNowSupervisor = supervisor
				runNowSupervisorSet = true
			}
		}
	}

	if runNow && cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(shutdownCtx, cfg, "scheduler-run-now", schedulerRunNowTelegram(shutdownCtx, cfg, db, runNowResearchSummary, runNowDailyOK, runNowReconcileOK, runNowSupervisor, runNowSupervisorSet, runNowNotes))
	}
	writeHeartbeat("scheduler ready")

	for {
		waitUntil := nextDaily
		if cfg.Live.Enabled && nextReconcile.Before(waitUntil) {
			waitUntil = nextReconcile
		}
		if cfg.Live.SupervisorEnabled && nextSupervisor.Before(waitUntil) {
			waitUntil = nextSupervisor
		}
		if researchScheduled && nextResearch.Before(waitUntil) {
			waitUntil = nextResearch
		}
		if monitoringEnabled && nextMarketWatch.Before(waitUntil) {
			waitUntil = nextMarketWatch
		}
		if maintenanceEnabled && nextMaintenance.Before(waitUntil) {
			waitUntil = nextMaintenance
		}
		if alivePingEnabled && nextAlivePing.Before(waitUntil) {
			waitUntil = nextAlivePing
		}
		if telegramCommandsEnabled && nextTelegramCommands.Before(waitUntil) {
			waitUntil = nextTelegramCommands
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
			heartbeat.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
			heartbeat.Status = "stopped"
			heartbeat.LastEvent = "scheduler stopped"
			heartbeat.LastEventAt = heartbeat.GeneratedAt
			if err := writeSchedulerHeartbeat(heartbeat); err != nil {
				log.Printf("[Scheduler] Heartbeat write warning: %v", err)
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
			// Run AI watch after daily analysis so it has fresh data.
			if cfg.AI.Enabled {
				log.Println("[Scheduler] Triggering scheduled AI watch (after daily run)...")
				aiCtx, cancel := context.WithTimeout(shutdownCtx, schedulerAIWatchTimeout)
				if err := runAIWatch(aiCtx, cfg, db); err != nil {
					log.Printf("[Scheduler] AI watch error: %v", err)
				}
				cancel()
			}
			now = time.Now().In(loc)
			nextDaily, err = getNextRunTime(dailyTime, loc, now)
			if err != nil {
				log.Printf("[Scheduler] Error calculating next run time: %v", err)
				nextDaily = now.Add(24 * time.Hour)
			}
			log.Printf("[Scheduler] Next scheduled daily run: %s", nextDaily.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("daily run completed")
		}

		if cfg.Live.Enabled && !time.Now().Before(nextReconcile) {
			log.Println("[Scheduler] Triggering scheduled live order reconciliation...")
			if err := runReconcileLiveOrders(shutdownCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Reconciliation error: %v", err)
			}
			nextReconcile = time.Now().Add(reconcileInterval)
			writeHeartbeat("live reconciliation completed")
		}

		if researchScheduled && !time.Now().Before(nextResearch) {
			log.Println("[Scheduler] Triggering scheduled research brief...")
			researchCtx, cancel := context.WithTimeout(shutdownCtx, schedulerResearchTimeout)
			if _, err := runResearchBrief(researchCtx, cfg, true); err != nil {
				log.Printf("[Scheduler] Research brief error: %v", err)
			}
			cancel()
			nextResearch = time.Now().Add(researchInterval)
			log.Printf("[Scheduler] Next research brief: %s", nextResearch.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("research brief completed")
		}

		if monitoringEnabled && !time.Now().Before(nextMarketWatch) {
			log.Println("[Scheduler] Triggering market watch cycle...")
			if report, err := runMarketWatch(shutdownCtx, cfg, db, true); err != nil {
				consecutiveMarketErrors++
				log.Printf("[Scheduler] Market watch error (%d): %v", consecutiveMarketErrors, err)
				threshold := cfg.Monitoring.MaxConsecutiveScanErrors
				if threshold <= 0 {
					threshold = 3
				}
				repeat := time.Duration(cfg.Monitoring.CriticalRepeatMinutes) * time.Minute
				if repeat <= 0 {
					repeat = time.Hour
				}
				alertDue := lastMarketErrorAlert.IsZero() || time.Since(lastMarketErrorAlert) >= repeat
				if consecutiveMarketErrors >= threshold && alertDue && cfg.Monitoring.NotifyOnCritical && cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
					sendTelegram(shutdownCtx, cfg, "market-watch-error", fmt.Sprintf("🚨 BTC Agent — Lỗi theo dõi thị trường\nLỗi liên tiếp: %d (ngưỡng %d)\nChi tiết: %s\nBot fail-closed: không mở lệnh mới cho tới khi dữ liệu hoạt động bình thường.", consecutiveMarketErrors, threshold, err))
					lastMarketErrorAlert = time.Now()
				}
				nextMarketWatch = time.Now().Add(marketWatchFailureDelay(marketScanInterval, consecutiveMarketErrors))
			} else {
				consecutiveMarketErrors = 0
				nextMarketWatch = time.Now().Add(marketWatchSuccessDelay(marketScanInterval, report))
			}
			log.Printf("[Scheduler] Next market watch: %s", nextMarketWatch.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("market watch completed")
		}

		if cfg.Live.SupervisorEnabled && !time.Now().Before(nextSupervisor) {
			log.Println("[Scheduler] Triggering scheduled live supervisor cycle...")
			doctor, err := runLiveDoctor(shutdownCtx, cfg, db)
			if err != nil {
				log.Printf("[Scheduler] Live doctor error: %v", err)
			} else {
				latestDoctor = &doctor
				log.Printf("[Scheduler] Live doctor status: %s", doctor.Summary)
				if shouldRefreshMarketDataForDoctor(doctor) {
					log.Println("[Scheduler] Live doctor found stale market data; refreshing analysis/plan before supervisor...")
					if err := runDailyWithNotify(shutdownCtx, cfg, db, false); err != nil {
						log.Printf("[Scheduler] Stale-data refresh error: %v", err)
					} else if refreshed, err := runLiveDoctor(shutdownCtx, cfg, db); err != nil {
						log.Printf("[Scheduler] Live doctor after stale-data refresh error: %v", err)
					} else {
						latestDoctor = &refreshed
						log.Printf("[Scheduler] Live doctor after stale-data refresh: %s", refreshed.Summary)
					}
				}
			}
			if latestDoctor != nil {
				consecutiveDoctorBlocks = updateDoctorBlockWatchdog(shutdownCtx, cfg, db, *latestDoctor, consecutiveDoctorBlocks)
			}
			if _, err := runLiveSupervisorCycleWithDoctor(shutdownCtx, cfg, db, &liveSupervisor, dryRun, latestDoctor); err != nil {
				log.Printf("[Scheduler] Live supervisor error: %v", err)
			}
			nextSupervisor = time.Now().Add(managementInterval)
			log.Printf("[Scheduler] Next live supervisor cycle: %s", nextSupervisor.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("live supervisor completed")
		}

		if alivePingEnabled && !time.Now().Before(nextAlivePing) {
			log.Println("[Scheduler] Sending alive ping to Telegram...")
			sendTelegram(shutdownCtx, cfg, "scheduler-alive", buildAlivePingText(heartbeat))
			nextAlivePing = time.Now().Add(alivePingInterval)
			writeHeartbeat("alive ping sent")
		}

		if telegramCommandsEnabled && !time.Now().Before(nextTelegramCommands) {
			if err := runTelegramCommands(shutdownCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Telegram commands warning: %v", err)
			}
			nextTelegramCommands = time.Now().Add(telegramCommandsInterval)
			writeHeartbeat("telegram commands polled")
		}

		if maintenanceEnabled && !time.Now().Before(nextMaintenance) {
			log.Println("[Scheduler] Triggering scheduled maintenance...")
			if err := runMaintenance(cfg, db); err != nil {
				log.Printf("[Scheduler] Maintenance error: %v", err)
			}
			// Run learning recommendations after maintenance so analysis rows are pruned first.
			log.Println("[Scheduler] Triggering scheduled learning (after maintenance)...")
			if err := runLearning(cfg, db); err != nil {
				log.Printf("[Scheduler] Learning error: %v", err)
			}
			now = time.Now().In(loc)
			nextMaintenance, err = getNextRunTime(maintenanceTime, loc, now)
			if err != nil {
				log.Printf("[Scheduler] Error calculating next maintenance run time: %v", err)
				nextMaintenance = now.Add(24 * time.Hour)
			}
			log.Printf("[Scheduler] Next scheduled maintenance run: %s", nextMaintenance.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("maintenance completed")
		}
	}
}
