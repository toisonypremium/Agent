package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/schedulerreport"
	"btc-agent/internal/storage"
	"btc-agent/internal/textsafe"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

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
		if !strings.Contains(lower, "mm=") && !strings.Contains(lower, "mm footprint") && !strings.Contains(lower, "dòng tiền lớn") && !strings.Contains(lower, "dấu chân dòng tiền lớn") {
			return fmt.Errorf("missing MM footprint detail")
		}
		if !strings.Contains(lower, "liq=") && !strings.Contains(lower, "liquidity") && !strings.Contains(lower, "thanh khoản:") && !strings.Contains(lower, "thanh khoản hạng") {
			return fmt.Errorf("missing liquidity detail")
		}
		if !strings.Contains(lower, "điều kiện") && !strings.Contains(lower, "trigger") && !strings.Contains(lower, "điều kiện mở khóa") && !strings.Contains(lower, "cần:") && !strings.Contains(lower, "chờ btc") {
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
	- Trước khi trả lời, tự kiểm: output có chứa đủ "I.", "II.", "III.", "IV.", "Dòng tiền lớn", "Thanh khoản", "Điều kiện", "không futures", "không leverage", "không market order".

	MẪU PHẢI BÁM SÁT, chỉ thay nội dung từ dữ liệu:
	📊 BTC Agent — Tóm tắt chiến lược
	I. KẾT LUẬN
	<1-2 câu: có đặt lệnh không; blocker chính; mode; BTC price/trend/regime/plan>

	II. BTC & KỊCH BẢN
	Xu hướng tuần/ngày/4 giờ: <...> | Dòng tiền <...> | rủi ro <...>
	Vùng giá: vùng mua <...> | hỗ trợ <...> | mốc sai kịch bản <...> | vùng cản <...>
	Kịch bản chính: <...>
	Kịch bản mở khóa: <...>
	Kịch bản vô hiệu: <...>
	Cần: <tối đa 4 điều kiện cụ thể>

	III. CƠ HỘI ĐANG THEO DÕI
	- <COIN> sẵn sàng <readiness>%% | Dòng tiền lớn: <case> <score>/100 (còn thiếu gì) | Thanh khoản: hạng <grade> <score>/100 | cách vùng mua <gap>%% | lãi/rủi ro <ratio> lần | Điều kiện tiếp theo: <next trigger>
	- Viết tương tự cho từng tài sản còn lại.

	IV. BOT & SAFETY
	Khi chưa đủ điều kiện đặt lệnh: không mua đuổi; tiếp tục chờ tín hiệu xác nhận.
	Vận hành: dự kiến=<...>, đã đặt=<...>, đã hủy=<...>, bị chặn=<...>.
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
