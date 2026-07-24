package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/dca"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

const (
	schedulerResearchTimeout   = 90 * time.Second
	schedulerAIWatchTimeout    = 120 * time.Second
	schedulerAITelegramTimeout = 90 * time.Second
	schedulerAuditTimeout      = 60 * time.Second
	schedulerHermesTimeout     = 90 * time.Second
)

func runScheduler(ctx context.Context, cfg config.Config, db *storage.DB, runNow bool, dryRun bool) error {
	if os.Getenv("BTC_AGENT_SCHEDULER_LOCK_HELD") != "true" {
		releaseLock, err := acquireSchedulerProcessLock()
		if err != nil {
			return err
		}
		defer releaseLock()
	}

	owner, ownedCtx, stopOwnership, err := startSchedulerOwnership(ctx, db)
	if err != nil {
		return err
	}
	defer stopOwnership()
	ctx = ownedCtx
	lease := owner.snapshot()
	log.Printf("[Scheduler] Execution owner instance=%s fencing=%d expires=%s", lease.InstanceID, lease.FencingToken, lease.ExpiresAt.Format(time.RFC3339))

	tz := cfg.App.Timezone
	if tz == "" {
		tz = "Asia/Ho_Chi_Minh"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", tz, err)
	}

	var dcaAllocator *dca.AllocationCoordinator
	log.Printf("[Scheduler] Started. Timezone: %s", tz)
	if cfg.DCA.AllocationEnabled {
		maxAge := time.Duration(cfg.DCA.ArtifactMaxAgeMinutes) * time.Minute
		if maxAge <= 0 {
			maxAge = 5 * time.Minute
		}
		dcaAllocator = &dca.AllocationCoordinator{DB: db, Source: dca.FileArtifactSource{Dir: cfg.DCA.ArtifactDirectory, MaxAge: maxAge}}
		log.Printf("[Scheduler] DCA allocation coordinator enabled; it has funding authority only, never order authority.")
	}
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

	dcaAllocationInterval := time.Minute
	log.Printf("[Scheduler] DCA allocation interval: %v (enabled: %v)", dcaAllocationInterval, dcaAllocator != nil)

	auditInterval := 60 * time.Minute
	if cfg.Live.AuditIntervalMinutes > 0 {
		auditInterval = time.Duration(cfg.Live.AuditIntervalMinutes) * time.Minute
	}
	auditEnabled := cfg.Live.SupervisorEnabled
	log.Printf("[Scheduler] Live auto-audit interval: %v (enabled: %v)", auditInterval, auditEnabled)

	hermesAvailable, hermesScheduled, hermesInterval := hermesSchedulePolicy(cfg)
	log.Printf("[Scheduler] Hermes cycle interval: %v (available: %v scheduled: %v)", hermesInterval, hermesAvailable, hermesScheduled)

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
	expertInterval := time.Duration(cfg.Research.ExpertIntervalMinutes) * time.Minute
	expertScheduled := cfg.Research.Enabled && cfg.Research.ExpertEnabled && expertInterval > 0
	log.Printf("[Scheduler] Expert research interval: %v scheduled=%v (enabled: %v)", expertInterval, expertScheduled, cfg.Research.ExpertEnabled)

	marketScanInterval := time.Duration(cfg.Monitoring.MarketScanIntervalMinutes) * time.Minute
	if marketScanInterval <= 0 {
		marketScanInterval = 15 * time.Minute
	}
	monitoringEnabled := cfg.Monitoring.Enabled
	log.Printf("[Scheduler] Market watch interval: %v (enabled: %v)", marketScanInterval, monitoringEnabled)

	var nextDaily time.Time
	var nextMaintenance time.Time
	var nextResearch time.Time
	var nextExpert time.Time
	var nextMarketWatch time.Time
	var nextReconcile time.Time
	var nextDCAAllocation time.Time
	var nextSupervisor time.Time
	var nextAudit time.Time
	var nextHermes time.Time
	var nextAlivePing time.Time
	var nextTelegramCommands time.Time
	var nextHermesOpening, nextHermesMidday, nextHermesClosing, nextHermesDigest time.Time
	var latestDoctor *liveguard.RuntimeDoctorResult
	consecutiveDoctorBlocks := 0
	consecutiveMarketErrors := 0
	var lastMarketErrorAlert time.Time

	telegramCommandsInterval := 30 * time.Second
	telegramBriefsEnabled := telegramBriefScheduleEnabled(cfg)
	telegramCommandsEnabled := telegramBriefsEnabled
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
		heartbeat.StartupPhase = ""
		heartbeat.PhaseStartedAt = ""
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
	writeStartupPhase := func(phase string) {
		now := time.Now().UTC().Format(time.RFC3339)
		if heartbeat.StartupPhase != "" {
			heartbeat.LastSuccessfulPhase = heartbeat.StartupPhase
		}
		heartbeat.GeneratedAt = now
		heartbeat.Status = "starting"
		heartbeat.StartupPhase = phase
		heartbeat.PhaseStartedAt = now
		heartbeat.LastEvent = "startup phase: " + phase
		heartbeat.LastEventAt = now
		if err := writeSchedulerHeartbeat(heartbeat); err != nil {
			log.Printf("[Scheduler] Startup heartbeat write warning: %v", err)
		}
	}

	// Calculate initial next daily run time
	nextDaily, err = getNextRunTime(dailyTime, loc, time.Now().In(loc))
	if err != nil {
		return err
	}
	if telegramBriefsEnabled {
		nextHermesOpening, _ = getNextRunTime("07:00", loc, time.Now().In(loc))
		nextHermesMidday, _ = getNextRunTime("13:00", loc, time.Now().In(loc))
		nextHermesClosing, _ = getNextRunTime("23:00", loc, time.Now().In(loc))
		nextHermesDigest = time.Now().Add(4 * time.Hour)
		log.Printf("[Scheduler] Hermes Telegram brief schedule: opening=%s midday=%s closing=%s digest=%s", nextHermesOpening.Format(time.RFC3339), nextHermesMidday.Format(time.RFC3339), nextHermesClosing.Format(time.RFC3339), nextHermesDigest.Format(time.RFC3339))
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
		writeStartupPhase("research")
		log.Println("[Scheduler] Executing initial research doctor/brief (--run-now)...")
		researchCtx, cancel := context.WithTimeout(shutdownCtx, schedulerResearchTimeout)
		if _, err := runResearchDoctor(researchCtx, cfg); err != nil {
			log.Printf("[Scheduler] Initial research doctor error: %v", err)
			runNowNotes = append(runNowNotes, "research doctor: "+err.Error())
		}
		if brief, err := runResearchBriefWithDB(researchCtx, cfg, db, false); err != nil {
			log.Printf("[Scheduler] Initial research brief error: %v", err)
			runNowNotes = append(runNowNotes, "research brief: "+err.Error())
		} else {
			runNowResearchSummary = brief.Summary
		}
		cancel()
	}

	if runNow {
		writeStartupPhase("daily")
		log.Println("[Scheduler] Executing initial daily run (--run-now)...")
		if err := runDailyWithNotify(shutdownCtx, cfg, db, false); err != nil {
			log.Printf("[Scheduler] Initial daily run error: %v", err)
			runNowNotes = append(runNowNotes, "daily: "+err.Error())
		} else {
			runNowDailyOK = true
		}
		// AI watch runs after daily so it has fresh analysis/plan.
		if expertScheduled {
			writeStartupPhase("expert_research")
			log.Println("[Scheduler] Executing initial expert research report (--run-now)...")
			expertCtx, cancel := context.WithTimeout(shutdownCtx, schedulerResearchTimeout)
			if err := runExpertResearch(expertCtx, cfg, db, false, true); err != nil {
				log.Printf("[Scheduler] Initial expert research error: %v", err)
				runNowNotes = append(runNowNotes, "expert research: "+err.Error())
			}
			cancel()
		}
		if cfg.AI.Enabled {
			writeStartupPhase("ai_watch")
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
	if expertScheduled {
		nextExpert = time.Now().Add(expertInterval)
		log.Printf("[Scheduler] Next expert research: %s", nextExpert.Format("2006-01-02 15:04:05 MST"))
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

	// Run reconciliation once on start if live is enabled. Recovery must fail
	// closed before any supervisor cycle can be scheduled after a restart.
	if cfg.Live.Enabled {
		writeStartupPhase("reconcile")
		log.Println("[Scheduler] Executing initial live order reconciliation...")
		notifyReconcile := !runNow
		reconcileErr := runReconcileLiveOrdersWithNotify(shutdownCtx, cfg, db, notifyReconcile)
		reconcileResult, reportOK := loadLatestReconcileReport()
		if reconcileErr != nil {
			log.Printf("[Scheduler] Initial reconciliation error: %v", reconcileErr)
			if err := enforceStartupReconcileRecovery(db, liveguard.ReconcileResult{}, reconcileErr); err != nil {
				return err
			}
			if runNow {
				runNowNotes = append(runNowNotes, "reconcile: "+reconcileErr.Error())
			}
		} else if !reportOK {
			missingErr := fmt.Errorf("initial reconcile report unavailable")
			log.Printf("[Scheduler] %v", missingErr)
			if err := enforceStartupReconcileRecovery(db, liveguard.ReconcileResult{}, missingErr); err != nil {
				return err
			}
			if runNow {
				runNowNotes = append(runNowNotes, "reconcile: "+missingErr.Error())
			}
		} else if err := enforceStartupReconcileRecovery(db, reconcileResult, nil); err != nil {
			return err
		} else if runNow {
			runNowReconcileOK = reconcileResult.Safety.Status == liveguard.ReconcileClean
		}
		nextReconcile = time.Now().Add(reconcileInterval)
	}

	if dcaAllocator != nil {
		nextDCAAllocation = time.Now()
		log.Printf("[Scheduler] DCA allocation coordinator scheduled now then every %v", dcaAllocationInterval)
	}

	liveSupervisor := liveSupervisorState{}
	if cfg.Live.SupervisorEnabled {
		writeStartupPhase("live_doctor")
		doctor, err := runLiveDoctor(shutdownCtx, cfg, db)
		if err != nil {
			log.Printf("[Scheduler] Live doctor error: %v", err)
			latestDoctor = unavailableDoctorResult(err)
		} else {
			latestDoctor = &doctor
			log.Printf("[Scheduler] Live doctor status: %s", doctor.Summary)
			if !dryRun && doctor.Status == liveguard.DoctorBlock {
				log.Printf("[Scheduler] Live supervisor real management blocked by doctor: %s", doctor.Summary)
			}
		}
		nextSupervisor = time.Now().Add(managementInterval)
		log.Printf("[Scheduler] Next live supervisor cycle: %s", nextSupervisor.Format("2006-01-02 15:04:05 MST"))
	}

	if auditEnabled {
		nextAudit = time.Now().Add(auditInterval)
		if runNow {
			writeStartupPhase("live_auto_audit")
			log.Println("[Scheduler] Executing initial live-auto-audit (--run-now)...")
			auditCtx, cancel := context.WithTimeout(shutdownCtx, schedulerAuditTimeout)
			if err := runLiveAutoAudit(auditCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Initial live-auto-audit error: %v", err)
				runNowNotes = append(runNowNotes, "live-auto-audit: "+err.Error())
			}
			cancel()
		}
		log.Printf("[Scheduler] Next live-auto-audit: %s", nextAudit.Format("2006-01-02 15:04:05 MST"))
	}

	if runNow && hermesAvailable {
		writeStartupPhase("hermes")
		log.Println("[Scheduler] Executing initial Hermes cycle (--run-now)...")
		hermesCtx, cancel := context.WithTimeout(shutdownCtx, schedulerHermesTimeout)
		if err := runHermesCycle(hermesCtx, cfg, db); err != nil {
			log.Printf("[Scheduler] Initial Hermes cycle error: %v", err)
			runNowNotes = append(runNowNotes, "hermes: "+err.Error())
		}
		cancel()
	}
	if hermesScheduled {
		nextHermes = time.Now().Add(hermesInterval)
		log.Printf("[Scheduler] Next Hermes cycle: %s", nextHermes.Format("2006-01-02 15:04:05 MST"))
	}

	if runNow && cfg.Live.SupervisorEnabled {
		writeStartupPhase("live_supervisor")
		if hermesAvailable && cfg.HermesOperator.CanExecute() {
			hermesPreCtx, hermesPreCancel := context.WithTimeout(shutdownCtx, schedulerHermesTimeout)
			if err := runHermesDecisionCycle(hermesPreCtx, cfg, db, hermesagent.HermesTrigger{Source: "supervisor", Reason: "pre_execution", AllowNotify: false}); err != nil {
				log.Printf("[Scheduler] Initial Hermes pre-execution decision error: %v", err)
				runNowNotes = append(runNowNotes, "Hermes pre-execution: "+err.Error())
			}
			hermesPreCancel()
			log.Println("[Scheduler] Hermes pre-execution decision gate completed before initial supervisor execution.")
		}
		log.Println("[Scheduler] Executing initial live supervisor cycle (--run-now) after audit/Hermes decision...")
		if supervisor, err := runLiveSupervisorCycleWithDoctorNotify(shutdownCtx, cfg, db, &liveSupervisor, dryRun, latestDoctor, false); err != nil {
			log.Printf("[Scheduler] Initial live supervisor error: %v", err)
			runNowNotes = append(runNowNotes, "supervisor: "+err.Error())
		} else {
			runNowSupervisor = supervisor
			runNowSupervisorSet = true
		}
	}

	if runNow && cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		writeStartupPhase("notification")
		sendScheduledTelegram(shutdownCtx, cfg, "scheduler-run-now", schedulerRunNowTelegram(shutdownCtx, cfg, db, runNowResearchSummary, runNowDailyOK, runNowReconcileOK, runNowSupervisor, runNowSupervisorSet, runNowNotes))
	}
	heartbeat.LastSuccessfulPhase = heartbeat.StartupPhase
	writeHeartbeat("scheduler ready")

	for {
		waitUntil := nextDaily
		if cfg.Live.Enabled && nextReconcile.Before(waitUntil) {
			waitUntil = nextReconcile
		}
		if dcaAllocator != nil && nextDCAAllocation.Before(waitUntil) {
			waitUntil = nextDCAAllocation
		}
		if cfg.Live.SupervisorEnabled && nextSupervisor.Before(waitUntil) {
			waitUntil = nextSupervisor
		}
		if researchScheduled && nextResearch.Before(waitUntil) {
			waitUntil = nextResearch
		}
		if expertScheduled && nextExpert.Before(waitUntil) {
			waitUntil = nextExpert
		}
		if monitoringEnabled && nextMarketWatch.Before(waitUntil) {
			waitUntil = nextMarketWatch
		}
		if maintenanceEnabled && nextMaintenance.Before(waitUntil) {
			waitUntil = nextMaintenance
		}
		if auditEnabled && nextAudit.Before(waitUntil) {
			waitUntil = nextAudit
		}
		if hermesScheduled && nextHermes.Before(waitUntil) {
			waitUntil = nextHermes
		}
		if alivePingEnabled && nextAlivePing.Before(waitUntil) {
			waitUntil = nextAlivePing
		}
		if telegramBriefsEnabled {
			if nextHermesOpening.Before(waitUntil) {
				waitUntil = nextHermesOpening
			}
			if nextHermesMidday.Before(waitUntil) {
				waitUntil = nextHermesMidday
			}
			if nextHermesClosing.Before(waitUntil) {
				waitUntil = nextHermesClosing
			}
			if nextHermesDigest.Before(waitUntil) {
				waitUntil = nextHermesDigest
			}
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

		if telegramBriefsEnabled {
			nowUTC := time.Now().UTC()
			if !nowUTC.Before(nextHermesOpening) {
				sendScheduledTelegram(shutdownCtx, cfg, "hermes-opening", renderHermesExecutive(buildHermesOperationsBrief(cfg, "opening brief")))
				nextHermesOpening, _ = getNextRunTime("07:00", loc, time.Now().In(loc))
			}
			if !nowUTC.Before(nextHermesMidday) {
				sendScheduledTelegram(shutdownCtx, cfg, "hermes-midday", renderHermesExecutive(buildHermesOperationsBrief(cfg, "mid-day review")))
				nextHermesMidday, _ = getNextRunTime("13:00", loc, time.Now().In(loc))
			}
			if !nowUTC.Before(nextHermesClosing) {
				sendScheduledTelegram(shutdownCtx, cfg, "hermes-closing", renderHermesExecutive(buildHermesOperationsBrief(cfg, "closing review")))
				nextHermesClosing, _ = getNextRunTime("23:00", loc, time.Now().In(loc))
			}
			if !nowUTC.Before(nextHermesDigest) {
				sendScheduledTelegram(shutdownCtx, cfg, "hermes-digest", renderHermesExecutive(buildHermesOperationsBrief(cfg, "4h digest")))
				nextHermesDigest = time.Now().Add(4 * time.Hour)
			}
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
			if _, err := runResearchBriefWithDB(researchCtx, cfg, db, true); err != nil {
				log.Printf("[Scheduler] Research brief error: %v", err)
			}
			cancel()
			nextResearch = time.Now().Add(researchInterval)
			log.Printf("[Scheduler] Next research brief: %s", nextResearch.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("research brief completed")
		}

		if expertScheduled && !time.Now().Before(nextExpert) {
			log.Println("[Scheduler] Triggering scheduled expert research report...")
			expertCtx, cancel := context.WithTimeout(shutdownCtx, schedulerResearchTimeout)
			if err := runExpertResearch(expertCtx, cfg, db, false, true); err != nil {
				log.Printf("[Scheduler] Expert research error: %v", err)
			}
			cancel()
			nextExpert = time.Now().Add(expertInterval)
			log.Printf("[Scheduler] Next expert research: %s", nextExpert.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("expert research completed")
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
					sendScheduledTelegram(shutdownCtx, cfg, "market-watch-error", fmt.Sprintf("🚨 BTC Agent — Lỗi theo dõi thị trường\nLỗi liên tiếp: %d (ngưỡng %d)\nChi tiết: %s\nBot fail-closed: không mở lệnh mới cho tới khi dữ liệu hoạt động bình thường.", consecutiveMarketErrors, threshold, err))
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

		if dcaAllocator != nil && !time.Now().Before(nextDCAAllocation) {
			if allocation, err := dcaAllocator.ObserveAndMaybeAllocate(); err != nil {
				log.Printf("[Scheduler] DCA allocation coordinator error: %v", err)
				if _, autoHalted, safetyErr := db.RecordDCASafetyCycle(true, false, "dca_coordinator_error", time.Now()); safetyErr != nil {
					log.Printf("[Scheduler] DCA safety accounting error: %v", safetyErr)
				} else if autoHalted {
					log.Printf("[Scheduler] DCA safety auto-halt active after three coordinator errors")
				}
			} else if allocation.EpochID > 0 {
				_, _, _ = db.RecordDCASafetyCycle(false, false, "", time.Now())
				log.Printf("[Scheduler] DCA allocation epoch=%d applied=%v", allocation.EpochID, allocation.Applied)
			} else {
				stale := allocation.Reason == "artifact_unavailable" || allocation.Reason == "artifact_not_verified"
				if _, autoHalted, safetyErr := db.RecordDCASafetyCycle(false, stale, allocation.Reason, time.Now()); safetyErr != nil {
					log.Printf("[Scheduler] DCA safety accounting error: %v", safetyErr)
				} else if autoHalted {
					log.Printf("[Scheduler] DCA safety auto-halt active: %s", allocation.Reason)
				}
				log.Printf("[Scheduler] DCA allocation not applied: %s", allocation.Reason)
			}
			nextDCAAllocation = time.Now().Add(dcaAllocationInterval)
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
			if hermesAvailable && cfg.HermesOperator.CanExecute() {
				hermesPreCtx, hermesPreCancel := context.WithTimeout(shutdownCtx, schedulerHermesTimeout)
				if err := runHermesDecisionCycle(hermesPreCtx, cfg, db, hermesagent.HermesTrigger{Source: "supervisor", Reason: "pre_execution", AllowNotify: false}); err != nil {
					log.Printf("[Scheduler] Hermes pre-execution decision error: %v", err)
				}
				hermesPreCancel()
			}
			if _, err := runLiveSupervisorCycleWithDoctor(shutdownCtx, cfg, db, &liveSupervisor, dryRun, latestDoctor); err != nil {
				log.Printf("[Scheduler] Live supervisor error: %v", err)
			}
			nextSupervisor = time.Now().Add(managementInterval)
			log.Printf("[Scheduler] Next live supervisor cycle: %s", nextSupervisor.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("live supervisor completed")
		}

		if auditEnabled && !time.Now().Before(nextAudit) {
			log.Println("[Scheduler] Triggering scheduled live-auto-audit...")
			auditCtx, cancel := context.WithTimeout(shutdownCtx, schedulerAuditTimeout)
			if err := runLiveAutoAudit(auditCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Live-auto-audit error: %v", err)
			}
			cancel()
			nextAudit = time.Now().Add(auditInterval)
			log.Printf("[Scheduler] Next live-auto-audit: %s", nextAudit.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("live-auto-audit completed")
		}

		if hermesScheduled && !time.Now().Before(nextHermes) {
			log.Println("[Scheduler] Triggering scheduled Hermes cycle...")
			hermesCtx, cancel := context.WithTimeout(shutdownCtx, schedulerHermesTimeout)
			if err := runHermesCycle(hermesCtx, cfg, db); err != nil {
				log.Printf("[Scheduler] Hermes cycle error: %v", err)
			}
			cancel()
			nextHermes = time.Now().Add(hermesInterval)
			log.Printf("[Scheduler] Next Hermes cycle: %s", nextHermes.Format("2006-01-02 15:04:05 MST"))
			writeHeartbeat("hermes cycle completed")
		}

		if alivePingEnabled && !time.Now().Before(nextAlivePing) {
			log.Println("[Scheduler] Sending alive ping to Telegram...")
			sendScheduledTelegram(shutdownCtx, cfg, "scheduler-alive", buildAlivePingText(heartbeat))
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

func loadLatestReconcileReport() (liveguard.ReconcileResult, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "live_reconcile_latest.json"))
	if err != nil {
		return liveguard.ReconcileResult{}, false
	}
	var result liveguard.ReconcileResult
	if json.Unmarshal(b, &result) != nil || result.Safety.Status == "" {
		return liveguard.ReconcileResult{}, false
	}
	return result, true
}

func enforceStartupReconcileRecovery(db *storage.DB, result liveguard.ReconcileResult, reconcileErr error) error {
	if reconcileErr == nil && result.Safety.Status != liveguard.ReconcileBlock {
		return nil
	}
	if err := db.SetHermesDemoted(true); err != nil {
		return fmt.Errorf("startup recovery demote Hermes: %w", err)
	}
	if err := db.SetHaltStatus(true); err != nil {
		return fmt.Errorf("startup recovery activate operator halt: %w", err)
	}
	reason := "reconcile_block"
	if reconcileErr != nil {
		reason = "reconcile_error"
	}
	payload, _ := json.Marshal(map[string]any{
		"reason":               reason,
		"reconcile_status":     result.Safety.Status,
		"unknown_orders":       result.Safety.Unknown,
		"remote_only":          result.Safety.RemoteOnly,
		"identity_conflicts":   result.Safety.IdentityConflicts,
		"discovery_failed":     result.Safety.DiscoveryFailed,
		"open_after_reconcile": result.Safety.OpenAfterReconcile,
		"unknown_positions":    result.Safety.UnknownPositions,
	})
	fingerprint := fmt.Sprintf("startup-recovery:%s:%s:%d:%d:%d:%t:%d:%d", reason, result.Safety.Status, result.Safety.Unknown, result.Safety.RemoteOnly, result.Safety.IdentityConflicts, result.Safety.DiscoveryFailed, result.Safety.OpenAfterReconcile, result.Safety.UnknownPositions)
	if err := db.SaveRuntimeEvent(storage.RuntimeEvent{Timestamp: time.Now().UTC(), Source: "btc-agent-scheduler", Type: "STARTUP_RECONCILE_RECOVERY_FAILED", Severity: "critical", Fingerprint: fingerprint, PayloadJSON: string(payload)}); err != nil {
		return fmt.Errorf("startup recovery save runtime event: %w", err)
	}
	log.Printf("[Scheduler] Startup recovery fail-closed: reason=%s status=%s; Hermes demoted and operator halt active", reason, result.Safety.Status)
	return nil
}

func unavailableDoctorResult(err error) *liveguard.RuntimeDoctorResult {
	r := &liveguard.RuntimeDoctorResult{GeneratedAt: time.Now().UTC(), Status: liveguard.DoctorBlock, Blockers: []string{"live doctor unavailable: " + err.Error()}}
	r.RefreshSummary()
	return r
}
