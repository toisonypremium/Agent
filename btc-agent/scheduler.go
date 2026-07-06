package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/schedulerreport"
	"btc-agent/internal/storage"
	"btc-agent/internal/textsafe"
)

func runScheduler(ctx context.Context, cfg config.Config, db *storage.DB, runNow bool, dryRun bool) error {
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
		if _, err := runResearchDoctor(shutdownCtx, cfg); err != nil {
			log.Printf("[Scheduler] Initial research doctor error: %v", err)
			runNowNotes = append(runNowNotes, "research doctor: "+err.Error())
		}
		if brief, err := runResearchBrief(shutdownCtx, cfg, false); err != nil {
			log.Printf("[Scheduler] Initial research brief error: %v", err)
			runNowNotes = append(runNowNotes, "research brief: "+err.Error())
		} else {
			runNowResearchSummary = brief.Summary
		}
	}

	if runNow {
		log.Println("[Scheduler] Executing initial daily run (--run-now)...")
		if err := runDailyWithNotify(shutdownCtx, cfg, db, false); err != nil {
			log.Printf("[Scheduler] Initial daily run error: %v", err)
			runNowNotes = append(runNowNotes, "daily: "+err.Error())
		} else {
			runNowDailyOK = true
		}
	}

	var nextResearch time.Time
	if researchScheduled {
		nextResearch = time.Now().Add(researchInterval)
		log.Printf("[Scheduler] Next research brief: %s", nextResearch.Format("2006-01-02 15:04:05 MST"))
	} else if cfg.Research.Enabled {
		// #8: interval=0 → run on --run-now only; no scheduled repeats.
		log.Printf("[Scheduler] Research enabled but brief_interval_minutes=0: no scheduled repeats.")
	}

	// Run reconciliation once on start if live is enabled, then schedule future ticks.
	var nextReconcile time.Time
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

	var nextSupervisor time.Time
	var latestDoctor *liveguard.RuntimeDoctorResult
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

		if researchScheduled && !time.Now().Before(nextResearch) {
			log.Println("[Scheduler] Triggering scheduled research brief...")
			if _, err := runResearchBrief(shutdownCtx, cfg, true); err != nil {
				log.Printf("[Scheduler] Research brief error: %v", err)
			}
			nextResearch = time.Now().Add(researchInterval)
			log.Printf("[Scheduler] Next research brief: %s", nextResearch.Format("2006-01-02 15:04:05 MST"))
		}

		if cfg.Live.SupervisorEnabled && !time.Now().Before(nextSupervisor) {
			log.Println("[Scheduler] Triggering scheduled live supervisor cycle...")
			doctor, err := runLiveDoctor(shutdownCtx, cfg, db)
			if err != nil {
				log.Printf("[Scheduler] Live doctor error: %v", err)
			} else {
				latestDoctor = &doctor
				log.Printf("[Scheduler] Live doctor status: %s", doctor.Summary)
			}
			if _, err := runLiveSupervisorCycleWithDoctor(shutdownCtx, cfg, db, &liveSupervisor, dryRun, latestDoctor); err != nil {
				log.Printf("[Scheduler] Live supervisor error: %v", err)
			}
			nextSupervisor = time.Now().Add(managementInterval)
			log.Printf("[Scheduler] Next live supervisor cycle: %s", nextSupervisor.Format("2006-01-02 15:04:05 MST"))
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

func schedulerRunNowTelegram(ctx context.Context, cfg config.Config, db *storage.DB, researchSummary string, dailyOK bool, reconcileOK bool, supervisor liveguard.SupervisorResult, supervisorSet bool, notes []string) string {
	fallback := schedulerRunNowTelegramDeterministic(db, researchSummary, dailyOK, reconcileOK, supervisor, supervisorSet, notes)
	if !cfg.AI.Enabled {
		return fallback
	}
	text, err := schedulerRunNowTelegramAI(ctx, cfg, db, researchSummary, dailyOK, reconcileOK, supervisor, supervisorSet, notes)
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
	if len(trimmed) < 1200 {
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
	prompt := fmt.Sprintf(`Viết 1 bản tin Telegram TIẾNG VIỆT như trader chuyên nghiệp báo cáo cho chủ tài khoản.
Không trả JSON. Không markdown fence. Không URL. Không tiếng Anh, trừ WATCH/ACTIVE_LIMIT/NO_TRADE.

BẮT BUỘC đủ 4 mục dưới đây, 1200-2400 ký tự:
	📊 BTC Agent — Tóm tắt chiến lược
	I. Kết luận: nói có đặt lệnh không, blocker chính là gì (1 dòng), mode hiện tại, BTC price/trend/regime/plan.
	II. BTC & Kịch bản: bias W/D/4H, flow, risk, vùng key (1 dòng), kịch bản chính/mở khóa/vô hiệu, điều kiện cần (tối đa 4 bullet).
	III. Watchlist MM/Liq: mỗi coin 1 dòng: COIN readiness%% | MM=<case> <score> (<top missing>) | Liq=<grade> <score> | gap <gap>%% RR <ratio> | <trigger>.
	IV. Bot & Safety: không ACTIVE_LIMIT thì nói rõ "không đặt lệnh, không chase", shadow nếu có, runtime desired/placed/canceled/blocked, research tối đa 1 câu, safety line.

	CẤM nội dung vô nghĩa:
	- Không viết "theo dõi thêm" nếu không có trigger cụ thể.
	- Không viết "thanh khoản chưa xác nhận" nếu không ghi Liq grade/score/reason.
	- Không viết "MM footprint chưa đủ" nếu không ghi MM case/missing item.
	- Không viết research dài hoặc link.
	Nếu không có ACTIVE_LIMIT, phải ghi rõ: không đặt lệnh, không chase giá, chờ trigger, điều kiện mở khóa. Không futures, không leverage, không market order.

	Dữ liệu:
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
