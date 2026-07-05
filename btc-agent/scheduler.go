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

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/storage"
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
	if strings.TrimSpace(text) == "" || len(strings.TrimSpace(text)) < 1200 || strings.Contains(strings.ToLower(text), "api_key") || strings.Contains(strings.ToLower(text), "telegram_token") {
		log.Printf("scheduler AI Telegram fallback: empty/short/unsafe output len=%d", len(strings.TrimSpace(text)))
		return fallback
	}
	log.Printf("scheduler AI Telegram ok (%d chars)", len(text))
	return strings.TrimSpace(text) + "\n"
}

func schedulerRunNowTelegramDeterministic(db *storage.DB, researchSummary string, dailyOK bool, reconcileOK bool, supervisor liveguard.SupervisorResult, supervisorSet bool, notes []string) string {
	analysis, analysisErr := db.LatestAnalysis()
	plan, planErr := db.LatestPlan()

	var b strings.Builder
	b.WriteString("📊 BTC Agent — Bản tin chiến lược\n")
	b.WriteString(time.Now().UTC().Format("02/01 15:04 UTC") + "\n")
	b.WriteString("───────────────────\n")

	if analysisErr != nil || planErr != nil {
		b.WriteString("Không đủ dữ liệu phân tích mới. Giữ nguyên trạng thái an toàn, không đặt lệnh.\n")
		b.WriteString("Research-only / system-only: không override Agent 1/2.\n")
		return strings.TrimSpace(b.String()) + "\n"
	}

	b.WriteString("I. KẾT LUẬN\n")
	b.WriteString(actionConclusionVI(analysis, plan))
	b.WriteString("\n")

	b.WriteString("II. PHÂN TÍCH KỸ THUẬT BTC\n")
	b.WriteString(fmt.Sprintf("Giá BTC: %.0f USDT | Regime: %s | Trend score: %.1f/100\n", analysis.BTCPrice, vietnameseRegime(analysis.MarketRegime), analysis.TrendScore))
	b.WriteString(fmt.Sprintf("Bias: tuần=%s, ngày=%s, 4H=%s\n", vietnameseBias(analysis.WeeklyBias), vietnameseBias(analysis.DailyBias), vietnameseBias(analysis.FourHourBias)))
	b.WriteString(fmt.Sprintf("Flow: %s %.2f — %s\n", analysis.Flow.Bias, analysis.Flow.Score, vietnameseFlowNote(fmt.Sprint(analysis.Flow.Bias))))
	b.WriteString(fmt.Sprintf("Rủi ro: tổng=%s | falling knife=%s | FOMO=%s\n", vietnameseRisk(analysis.RiskLevel), vietnameseRisk(analysis.FallingKnifeRisk), vietnameseRisk(analysis.FomoRisk)))
	b.WriteString("\nVùng giá quan trọng:\n")
	writeZoneVI(&b, "Gom", analysis.AccumulationZone.Low, analysis.AccumulationZone.High)
	writeZoneVI(&b, "Support chính", analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High)
	writeZoneVI(&b, "Support sâu", analysis.DeepSupportZone.Low, analysis.DeepSupportZone.High)
	writeZoneVI(&b, "Kháng cự", analysis.ResistanceZone.Low, analysis.ResistanceZone.High)
	writeZoneVI(&b, "Invalidation", analysis.InvalidationZone.Low, analysis.InvalidationZone.High)
	b.WriteString("───────────────────\n")

	b.WriteString("III. KỊCH BẢN THỊ TRƯỜNG\n")
	if analysis.ScenarioMain != "" {
		b.WriteString("Chính: " + analysis.ScenarioMain + "\n")
	}
	if analysis.ScenarioBullish != "" {
		b.WriteString("Tốt: " + analysis.ScenarioBullish + "\n")
	}
	if analysis.ScenarioBearish != "" {
		b.WriteString("Xấu: " + analysis.ScenarioBearish + "\n")
	}
	b.WriteString("───────────────────\n")

	b.WriteString("IV. KẾ HOẠCH BOT\n")
	b.WriteString(fmt.Sprintf("Permission: %s | Plan: %s\n", vietnamesePermission(analysis.ActionPermission), vietnamesePlanState(plan.State)))
	active := activeAssetsVI(plan)
	if len(active) > 0 {
		b.WriteString("Coin đủ điều kiện ACTIVE_LIMIT:\n")
		for _, asset := range active {
			b.WriteString(fmt.Sprintf("- %s | RR %.1f | rank #%d\n", asset.Symbol, asset.RewardRisk, asset.RotationRank))
			for _, layer := range asset.Layers {
				b.WriteString(fmt.Sprintf("  Layer %d: %.4f × %.2f USDT\n", layer.Index, layer.Price, layer.Notional))
			}
		}
	} else {
		b.WriteString("Chưa có coin ACTIVE_LIMIT. Bot không đặt lệnh.\n")
	}
	if len(plan.Watchlist.Candidates) > 0 {
		b.WriteString("Watchlist gần đạt:\n")
		limit := len(plan.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range plan.Watchlist.Candidates[:limit] {
			b.WriteString(fmt.Sprintf("- %s: readiness %.0f%% | chờ: %s\n", c.Symbol, c.ReadinessScore*100, emptyScheduler(c.NextTrigger, "thêm xác nhận")))
		}
	}
	b.WriteString("───────────────────\n")

	b.WriteString("V. TIN TỨC / RESEARCH\n")
	b.WriteString(emptyScheduler(researchSummary, "Không có research mới") + "\n")
	b.WriteString("Tin tức chỉ là bối cảnh, không dùng để tự đặt lệnh.\n")
	b.WriteString("───────────────────\n")

	b.WriteString("VI. TRẠNG THÁI THỰC THI\n")
	b.WriteString(fmt.Sprintf("Daily: %s | Reconcile: %s\n", okWarnVI(dailyOK), okWarnVI(reconcileOK)))
	if supervisorSet {
		b.WriteString(fmt.Sprintf("Supervisor: %s | Action: %s\n", supervisor.Status, supervisor.Action))
		if supervisor.Managed != nil {
			m := supervisor.Managed
			b.WriteString(fmt.Sprintf("Orders: desired=%d đặt=%d hủy=%d thay=%d chặn=%d\n", len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
			b.WriteString(fmt.Sprintf("Gates: data=%s | reconcile=%s | risk=%s\n", m.DataHealth.Status, m.ReconcileSafety.Status, m.RiskGovernor.Status))
		}
	}
	if len(notes) > 0 {
		b.WriteString("Cảnh báo hệ thống: " + strings.Join(notes, "; ") + "\n")
	}
	b.WriteString("\nAn toàn: không futures, không leverage, không market order. Chỉ spot limit BUY post-only khi Agent 2 ACTIVE_LIMIT và safety gate sạch.\n")
	return strings.TrimSpace(b.String()) + "\n"
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
			"price":              analysis.BTCPrice,
			"regime":             analysis.MarketRegime,
			"trend_score":        analysis.TrendScore,
			"weekly_bias":        analysis.WeeklyBias,
			"daily_bias":         analysis.DailyBias,
			"four_hour_bias":     analysis.FourHourBias,
			"flow_bias":          analysis.Flow.Bias,
			"flow_score":         analysis.Flow.Score,
			"risk_level":         analysis.RiskLevel,
			"falling_knife_risk": analysis.FallingKnifeRisk,
			"fomo_risk":          analysis.FomoRisk,
			"accumulation_zone":  analysis.AccumulationZone,
			"support_zone":       analysis.PrimarySupportZone,
			"deep_support_zone":  analysis.DeepSupportZone,
			"resistance_zone":    analysis.ResistanceZone,
			"invalidation_zone":  analysis.InvalidationZone,
			"scenario_main":      analysis.ScenarioMain,
			"scenario_bullish":   analysis.ScenarioBullish,
			"scenario_bearish":   analysis.ScenarioBearish,
			"permission":         analysis.ActionPermission,
		},
		"plan":               compactPlanForAI(plan),
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

BẮT BUỘC đủ 6 mục dưới đây, 1600-2600 ký tự:
📊 BTC Agent — Bản tin chiến lược
I. Kết luận: nói có đặt lệnh không, vì sao.
II. Phân tích kỹ thuật BTC: giá, regime, trend score, bias tuần/ngày/4H, flow score, risk.
III. Vùng giá & kịch bản: vùng gom, support, deep support, kháng cự, invalidation; kịch bản chính/tốt/xấu.
IV. Kế hoạch bot: permission, plan state, ACTIVE_LIMIT layer nếu có; nếu không có thì nói thiếu gì và watchlist chờ trigger nào.
V. Research context: chỉ bối cảnh phụ, không override.
VI. Trạng thái an toàn: daily/reconcile/supervisor/gates, kết luận spot limit BUY post-only only.

Nếu không có ACTIVE_LIMIT, phải ghi rõ: không đặt lệnh, không chase giá, chờ trigger. Không futures, không leverage, không market order.

Dữ liệu:
%s`, string(b))
	text, err := client.ChatText(ctx, prompt)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(text, "```"), "```"))
	text = stripURLsScheduler(text)
	if len(text) > 3200 {
		text = strings.TrimSpace(text[:3200]) + "\n..."
	}
	return text, nil
}

func compactPlanForAI(plan agent2.Plan) map[string]any {
	assets := []map[string]any{}
	for _, a := range plan.Assets {
		layers := []map[string]any{}
		for _, l := range a.Layers {
			layers = append(layers, map[string]any{"index": l.Index, "price": l.Price, "notional": l.Notional})
		}
		assets = append(assets, map[string]any{
			"symbol": a.Symbol, "state": a.State, "reason": a.Reason,
			"rotation_rank": a.RotationRank, "rotation_score": a.RotationScore,
			"reward_risk": a.RewardRisk, "layers": layers,
		})
	}
	watch := []map[string]any{}
	limit := len(plan.Watchlist.Candidates)
	if limit > 5 {
		limit = 5
	}
	for _, c := range plan.Watchlist.Candidates[:limit] {
		watch = append(watch, map[string]any{
			"symbol": c.Symbol, "readiness_score": c.ReadinessScore, "tier": c.Tier,
			"actionable": c.Actionable, "missing": c.Missing, "next_trigger": c.NextTrigger,
		})
	}
	return map[string]any{"state": plan.State, "summary": plan.Summary, "assets": assets, "watchlist": watch}
}

func stripURLsScheduler(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		words := strings.Fields(line)
		kept := []string{}
		for _, word := range words {
			lower := strings.ToLower(strings.Trim(word, "()[]{}.,;"))
			if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
				continue
			}
			kept = append(kept, word)
		}
		lines[i] = strings.Join(kept, " ")
	}
	return strings.Join(lines, "\n")
}

func actionConclusionVI(analysis agent1.MarketAnalysis, plan agent2.Plan) string {
	switch {
	case analysis.ActionPermission == agent1.NoTrade:
		return "Không giao dịch. BTC chưa cho phép, ưu tiên giữ USDT và chờ cấu trúc rõ hơn."
	case analysis.ActionPermission == agent1.Watch:
		return "Chỉ quan sát. Có thể theo dõi vùng support/discount, nhưng chưa đủ điều kiện đặt lệnh."
	case analysis.ActionPermission == agent1.Armed:
		return "Chuẩn bị. BTC gần đủ điều kiện, cần trigger rõ để chuyển sang ACTIVE_LIMIT."
	case analysis.ActionPermission == agent1.Allowed && plan.State == agent2.StateActiveLimit:
		return "Có setup được phép. Bot chỉ đặt spot limit BUY post-only nếu proof và safety gate sạch."
	case analysis.ActionPermission == agent1.Allowed:
		return "BTC đã cho phép tìm setup, nhưng Agent 2 chưa có layer ACTIVE_LIMIT. Không chase giá."
	default:
		return "Giữ an toàn, chờ Agent 1/2 xác nhận thêm."
	}
}

func vietnameseRegime(regime string) string {
	switch regime {
	case "UPTREND":
		return "xu hướng tăng"
	case "DOWNTREND":
		return "xu hướng giảm"
	case "RANGING":
		return "đi ngang"
	case "PANIC_SELLING":
		return "bán tháo"
	case "RECOVERY":
		return "phục hồi"
	default:
		return emptyScheduler(regime, "chưa rõ")
	}
}

func vietnameseBias(bias string) string {
	switch strings.ToUpper(bias) {
	case "BULLISH":
		return "tăng"
	case "BEARISH":
		return "giảm"
	case "NEUTRAL":
		return "trung lập"
	case "ACCUMULATION":
		return "tích lũy"
	case "DISTRIBUTION":
		return "phân phối"
	case "BEAR_TRAP":
		return "bear trap/reclaim"
	case "BULL_TRAP":
		return "bull trap"
	default:
		return emptyScheduler(bias, "chưa rõ")
	}
}

func vietnameseFlowNote(flow string) string {
	switch flow {
	case "ACCUMULATION":
		return "có lực gom, nhưng vẫn cần xác nhận từ vùng giá và risk gate."
	case "BEAR_TRAP":
		return "có tín hiệu rũ bỏ rồi reclaim; tốt nếu giữ được support."
	case "DISTRIBUTION":
		return "có dấu hiệu phân phối; không nên mua đuổi."
	case "BULL_TRAP":
		return "cẩn thận bẫy tăng; chờ retest."
	case "NEUTRAL":
		return "dòng tiền chưa rõ, chưa đủ làm trigger."
	default:
		return "cần thêm xác nhận."
	}
}

func vietnameseRisk(r agent1.Risk) string {
	switch r {
	case agent1.Low:
		return "thấp"
	case agent1.Medium:
		return "vừa"
	case agent1.High:
		return "cao"
	default:
		return string(r)
	}
}

func vietnamesePermission(p agent1.Permission) string {
	switch p {
	case agent1.Allowed:
		return "được phép tìm setup"
	case agent1.Armed:
		return "gần đủ điều kiện"
	case agent1.Watch:
		return "chỉ theo dõi"
	case agent1.NoTrade:
		return "không giao dịch"
	default:
		return string(p)
	}
}

func vietnamesePlanState(state agent2.State) string {
	switch state {
	case agent2.StateActiveLimit:
		return "ACTIVE_LIMIT — có layer hợp lệ"
	case agent2.StateArmed:
		return "ARMED — chờ trigger"
	case agent2.StateWatch:
		return "WATCH — chưa đặt lệnh"
	case agent2.StateNoTrade:
		return "NO_TRADE — đứng ngoài"
	default:
		return string(state)
	}
}

func writeZoneVI(b *strings.Builder, label string, low, high float64) {
	if low > 0 || high > 0 {
		b.WriteString(fmt.Sprintf("- %s: %.0f – %.0f\n", label, low, high))
	}
}

func activeAssetsVI(plan agent2.Plan) []agent2.AssetPlan {
	out := []agent2.AssetPlan{}
	for _, asset := range plan.Assets {
		if asset.State == agent2.StateActiveLimit {
			out = append(out, asset)
		}
	}
	return out
}

func okWarnVI(ok bool) string {
	if ok {
		return "OK"
	}
	return "WARN"
}

func emptyScheduler(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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
