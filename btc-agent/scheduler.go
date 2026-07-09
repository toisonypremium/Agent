package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/reportio"
	"btc-agent/internal/schedulerreport"
	"btc-agent/internal/storage"
	"btc-agent/internal/textsafe"
)

const (
	schedulerResearchTimeout   = 90 * time.Second
	schedulerAIWatchTimeout    = 120 * time.Second
	schedulerAITelegramTimeout = 90 * time.Second
)

type SchedulerHeartbeat struct {
	GeneratedAt             string `json:"generated_at"`
	PID                     int    `json:"pid"`
	Status                  string `json:"status"`
	Timezone                string `json:"timezone"`
	Mode                    string `json:"mode"`
	DryRun                  bool   `json:"dry_run"`
	LiveEnabled             bool   `json:"live_enabled"`
	LiveSupervisorEnabled   bool   `json:"live_supervisor_enabled"`
	ResearchEnabled         bool   `json:"research_enabled"`
	MaintenanceEnabled      bool   `json:"maintenance_enabled"`
	NextDailyRun            string `json:"next_daily_run,omitempty"`
	NextMaintenanceRun      string `json:"next_maintenance_run,omitempty"`
	NextResearchBrief       string `json:"next_research_brief,omitempty"`
	NextReconcile           string `json:"next_reconcile,omitempty"`
	NextLiveSupervisorCycle string `json:"next_live_supervisor_cycle,omitempty"`
	LastEvent               string `json:"last_event,omitempty"`
	LastEventAt             string `json:"last_event_at,omitempty"`
	DoctorStatus            string `json:"doctor_status,omitempty"`
	DoctorSummary           string `json:"doctor_summary,omitempty"`
	ConsecutiveDoctorBlocks int    `json:"consecutive_doctor_blocks"`
}

func writeSchedulerHeartbeat(h SchedulerHeartbeat) error {
	if h.GeneratedAt == "" {
		h.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return reportio.WriteJSON("reports", "scheduler_heartbeat_latest.json", h)
}

func schedulerHeartbeatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func acquireSchedulerProcessLock() (func(), error) {
	path := os.Getenv("BTC_AGENT_SCHEDULER_LOCK_FILE")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("scheduler lock home: %w", err)
		}
		path = filepath.Join(home, ".btc-agent-scheduler.lock")
	}
	if b, err := os.ReadFile(path); err == nil {
		pidText := strings.TrimSpace(string(b))
		if pid, err := strconv.Atoi(pidText); err == nil && pid > 0 {
			if err := syscall.Kill(pid, 0); err == nil {
				return nil, fmt.Errorf("btc-agent scheduler already running pid=%d", pid)
			}
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("write scheduler lock: %w", err)
	}
	return func() {
		b, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
			_ = os.Remove(path)
		}
	}, nil
}

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

	var nextDaily time.Time
	var nextMaintenance time.Time
	var nextResearch time.Time
	var nextReconcile time.Time
	var nextSupervisor time.Time
	var latestDoctor *liveguard.RuntimeDoctorResult
	consecutiveDoctorBlocks := 0

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
		heartbeat.NextReconcile = schedulerHeartbeatTime(nextReconcile)
		heartbeat.NextLiveSupervisorCycle = schedulerHeartbeatTime(nextSupervisor)
		if latestDoctor != nil {
			heartbeat.DoctorStatus = string(latestDoctor.Status)
			heartbeat.DoctorSummary = latestDoctor.Summary
		}
		heartbeat.ConsecutiveDoctorBlocks = consecutiveDoctorBlocks
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

func shouldRefreshMarketDataForDoctor(doctor liveguard.RuntimeDoctorResult) bool {
	if doctor.Status != liveguard.DoctorBlock {
		return false
	}
	for _, blocker := range doctor.Blockers {
		b := strings.ToLower(blocker)
		if strings.Contains(b, "analysis stale") || strings.Contains(b, "plan stale") || strings.Contains(b, "1d candle") {
			return true
		}
	}
	if doctor.DataHealth.Status == liveguard.DataHealthBlock {
		for _, blocker := range doctor.DataHealth.Blockers {
			b := strings.ToLower(blocker)
			if strings.Contains(b, "analysis stale") || strings.Contains(b, "plan stale") || strings.Contains(b, "1d candle") {
				return true
			}
		}
	}
	return false
}

func updateDoctorBlockWatchdog(ctx context.Context, cfg config.Config, db *storage.DB, doctor liveguard.RuntimeDoctorResult, consecutive int) int {
	if doctor.Status != liveguard.DoctorBlock {
		if consecutive > 0 {
			log.Printf("[Scheduler] Live doctor block watchdog reset after status=%s", doctor.Status)
		}
		return 0
	}
	consecutive++
	threshold := cfg.Live.AutoHaltAfterErrors
	log.Printf("[Scheduler] Live doctor block watchdog: consecutive=%d threshold=%d summary=%s", consecutive, threshold, doctor.Summary)
	if threshold <= 0 || consecutive < threshold {
		return consecutive
	}
	halted, err := db.IsHalted()
	if err != nil {
		log.Printf("[Scheduler] Live doctor block watchdog halt check error: %v", err)
		return consecutive
	}
	if halted {
		log.Printf("[Scheduler] Live doctor block watchdog threshold reached; operator halt already active")
		return consecutive
	}
	if err := db.SetHaltStatus(true); err != nil {
		log.Printf("[Scheduler] Live doctor block watchdog set halt error: %v", err)
		return consecutive
	}
	text := fmt.Sprintf("Operator halt: ACTIVE (auto-halt after %d consecutive live-doctor blocks). Last doctor: %s", consecutive, doctor.Summary)
	log.Printf("[Scheduler] %s", text)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "operator-halt", text)
	}
	return consecutive
}

func schedulerRunNowTelegram(ctx context.Context, cfg config.Config, db *storage.DB, researchSummary string, dailyOK bool, reconcileOK bool, supervisor liveguard.SupervisorResult, supervisorSet bool, notes []string) string {
	fallback := schedulerRunNowTelegramDeterministic(db, researchSummary, dailyOK, reconcileOK, supervisor, supervisorSet, notes)
	if !cfg.AI.Enabled {
		return fallback
	}
	aiCtx, cancel := context.WithTimeout(ctx, schedulerAITelegramTimeout)
	defer cancel()
	text, err := schedulerRunNowTelegramAI(aiCtx, cfg, db, researchSummary, dailyOK, reconcileOK, supervisor, supervisorSet, notes)
	if err != nil {
		log.Printf("scheduler AI Telegram fallback: %v", err)
		return fallback
	}
	if err := validateSchedulerTelegramAI(text); err != nil {
		log.Printf("scheduler AI Telegram fallback: %v len=%d", err, len(strings.TrimSpace(text)))
		return fallback
	}
	log.Printf("scheduler AI Telegram ok (%d chars)", len(text))
	return strings.TrimSpace(text) + "\n"
}

func validateSchedulerTelegramAI(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("empty output")
	}
	if len(trimmed) < 600 {
		return fmt.Errorf("short output")
	}
	if textsafe.ContainsSecretLike(trimmed) {
		return fmt.Errorf("unsafe secret-like output")
	}
	for _, want := range []string{"I.", "II.", "III.", "IV."} {
		if !strings.Contains(trimmed, want) {
			return fmt.Errorf("missing section %s", want)
		}
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "spot limit") || !strings.Contains(lower, "post-only") || !strings.Contains(lower, "không futures") || !strings.Contains(lower, "không leverage") || !strings.Contains(lower, "không market") {
		return fmt.Errorf("missing complete safety line")
	}
	if strings.Contains(trimmed, "http://") || strings.Contains(trimmed, "https://") {
		return fmt.Errorf("contains URL")
	}
	if strings.Contains(lower, "watch") || strings.Contains(lower, "không đặt lệnh") || strings.Contains(lower, "khong dat lenh") {
		if !strings.Contains(lower, "mm=") && !strings.Contains(lower, "mm footprint") {
			return fmt.Errorf("missing MM footprint detail")
		}
		if !strings.Contains(lower, "liq=") && !strings.Contains(lower, "liquidity") {
			return fmt.Errorf("missing liquidity detail")
		}
		if !strings.Contains(lower, "trigger") && !strings.Contains(lower, "điều kiện mở khóa") && !strings.Contains(lower, "cần:") && !strings.Contains(lower, "chờ btc") {
			return fmt.Errorf("missing actionable trigger")
		}
	}
	if strings.HasSuffix(trimmed, "...") || strings.HasSuffix(trimmed, "…") {
		return fmt.Errorf("truncated output")
	}
	return nil
}

func schedulerRunNowTelegramDeterministic(db *storage.DB, researchSummary string, dailyOK bool, reconcileOK bool, supervisor liveguard.SupervisorResult, supervisorSet bool, notes []string) string {
	analysis, analysisErr := db.LatestAnalysis()
	plan, planErr := db.LatestPlan()
	if analysisErr != nil || planErr != nil {
		return schedulerreport.BuildMissingData()
	}
	shadow, _ := liveguard.LoadShadowProbeLatest("reports/shadow_probe_latest.json")
	return schedulerreport.BuildDeterministic(schedulerreport.RunNowSnapshot{
		GeneratedAt:     time.Now().UTC(),
		Analysis:        analysis,
		Plan:            plan,
		ResearchSummary: researchSummary,
		DailyOK:         dailyOK,
		ReconcileOK:     reconcileOK,
		Supervisor:      supervisor,
		SupervisorSet:   supervisorSet,
		ShadowProbe:     shadow,
		Notes:           notes,
	})
}

func schedulerRunNowTelegramAI(ctx context.Context, cfg config.Config, db *storage.DB, researchSummary string, dailyOK bool, reconcileOK bool, supervisor liveguard.SupervisorResult, supervisorSet bool, notes []string) (string, error) {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return "", fmt.Errorf("latest analysis: %w", err)
	}
	plan, err := db.LatestPlan()
	if err != nil {
		return "", fmt.Errorf("latest plan: %w", err)
	}
	shadow, _ := liveguard.LoadShadowProbeLatest("reports/shadow_probe_latest.json")
	maxTokens := cfg.AI.MaxTokens
	if maxTokens < 2000 {
		maxTokens = 2000
	}
	client, err := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, maxTokens, cfg.AI.Temperature)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"btc": map[string]any{
			"price":             analysis.BTCPrice,
			"regime":            analysis.MarketRegime,
			"trend_score":       analysis.TrendScore,
			"score_breakdown":   analysis.ScoreBreakdown,
			"permission_reason": analysis.PermissionReason,
			"bias":              map[string]any{"weekly": analysis.WeeklyBias, "daily": analysis.DailyBias, "4h": analysis.FourHourBias},
			"flow":              map[string]any{"bias": analysis.Flow.Bias, "score": analysis.Flow.Score, "daily_components": analysis.Flow.Daily.Components, "daily_diagnostics": analysis.Flow.Daily.Diagnostics},
			"risk":              map[string]any{"level": analysis.RiskLevel, "falling_knife": analysis.FallingKnifeRisk, "fomo": analysis.FomoRisk},
			"zones":             map[string]any{"active_accumulation": analysis.AccumulationZone, "macro_accumulation": analysis.MacroAccumulationZone, "support": analysis.PrimarySupportZone, "deep_support": analysis.DeepSupportZone, "resistance": analysis.ResistanceZone, "invalidation": analysis.InvalidationZone},
			"scenarios":         map[string]string{"main": analysis.ScenarioMain, "bullish": analysis.ScenarioBullish, "bearish": analysis.ScenarioBearish},
			"permission":        analysis.ActionPermission,
		},
		"plan":               schedulerreport.CompactPlan(plan),
		"shadow_probe":       shadow,
		"research_summary":   researchSummary,
		"daily_ok":           dailyOK,
		"reconcile_ok":       reconcileOK,
		"supervisor_set":     supervisorSet,
		"supervisor_status":  supervisor.Status,
		"supervisor_action":  supervisor.Action,
		"supervisor_summary": supervisor.Summary,
		"notes":              notes,
	}
	if supervisor.Managed != nil {
		m := supervisor.Managed
		payload["managed"] = map[string]any{
			"status":               m.Status,
			"summary":              m.Summary,
			"desired":              len(m.Desired),
			"placed":               len(m.Placed),
			"canceled":             len(m.Canceled),
			"replaced":             len(m.Replaced),
			"blocked":              len(m.Blocked),
			"data_health":          m.DataHealth.Status,
			"reconcile_safety":     m.ReconcileSafety.Status,
			"risk_governor":        m.RiskGovernor.Status,
			"risk_warnings":        m.RiskGovernor.Warnings,
			"why_no_order_by_coin": m.PerCoin,
		}
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	prompt := fmt.Sprintf(`Bạn là bộ tạo tin Telegram. Nhiệm vụ: viết đúng format, không sáng tạo cấu trúc.
	Ngôn ngữ: TIẾNG VIỆT. Không JSON. Không markdown fence. Không URL. Một tin duy nhất.

	QUY TẮC FORMAT BẮT BUỘC:
	- Bắt đầu bằng đúng dòng: 📊 BTC Agent — Tóm tắt chiến lược
	- Phải có đủ 4 nhãn literal, đúng chữ, đúng thứ tự: I. II. III. IV.
	- Không được bỏ mục III. Không được gộp Watchlist vào mục khác.
	- Mỗi mục ngắn, tổng 1200-2400 ký tự.
	- Trước khi trả lời, tự kiểm: output có chứa đủ "I.", "II.", "III.", "IV.", "MM=", "Liq=", "trigger", "không futures", "không leverage", "không market order".

	MẪU PHẢI BÁM SÁT, chỉ thay nội dung từ dữ liệu:
	📊 BTC Agent — Tóm tắt chiến lược
	I. KẾT LUẬN
	<1-2 câu: có đặt lệnh không; blocker chính; mode; BTC price/trend/regime/plan>

	II. BTC & KỊCH BẢN
	Bias W/D/4H: <...> | Flow <...> | risk <...>
	Vùng: active <...> | support <...> | invalid <...> | resist <...>
	Kịch bản chính: <...>
	Kịch bản mở khóa: <...>
	Kịch bản vô hiệu: <...>
	Cần: <tối đa 4 điều kiện cụ thể>

	III. WATCHLIST MM/LIQ
	- <COIN> <readiness>%% | MM=<case> <score>/100 (<top missing>) | Liq=<grade> <score>/100 (<top reason nếu có>) | gap <gap>%% RR <ratio> | trigger=<next trigger>
	- <COIN> <readiness>%% | MM=<case> <score>/100 (<top missing>) | Liq=<grade> <score>/100 (<top reason nếu có>) | gap <gap>%% RR <ratio> | trigger=<next trigger>
	- <COIN> <readiness>%% | MM=<case> <score>/100 (<top missing>) | Liq=<grade> <score>/100 (<top reason nếu có>) | gap <gap>%% RR <ratio> | trigger=<next trigger>

	IV. BOT & SAFETY
	Không ACTIVE_LIMIT: không đặt lệnh, không chase; chờ điều kiện mở khóa.
	Runtime: desired=<...> placed=<...> canceled=<...> blocked=<...>.
	Research: <1 câu ngắn, context only>.
	An toàn: spot limit BUY post-only only; không futures, không leverage, không market order.

	CẤM:
	- Không viết "theo dõi thêm" nếu không có trigger cụ thể.
	- Không viết "thanh khoản chưa xác nhận" nếu không ghi Liq grade/score/reason.
	- Không viết "MM footprint chưa đủ" nếu không ghi MM case/missing item.
	- Không viết research dài hoặc link.
	- Không dùng bullet ngoài 4 mục trên.

	Dữ liệu duy nhất được phép dùng:
%s`, string(b))
	text, err := client.ChatText(ctx, prompt)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(text, "```"), "```"))
	text = textsafe.StripURLs(text)
	text = textsafe.TrimAtBoundary(text, 3400)
	return text, nil
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
