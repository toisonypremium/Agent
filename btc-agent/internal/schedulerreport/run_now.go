package schedulerreport

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
)

type RunNowSnapshot struct {
	GeneratedAt     time.Time
	Analysis        agent1.MarketAnalysis
	Plan            agent2.Plan
	ResearchSummary string
	DailyOK         bool
	ReconcileOK     bool
	Supervisor      liveguard.SupervisorResult
	SupervisorSet   bool
	ShadowProbe     liveguard.ShadowProbeJournal
	Notes           []string
}

func BuildDeterministic(s RunNowSnapshot) string {
	if s.GeneratedAt.IsZero() {
		s.GeneratedAt = time.Now().UTC()
	}
	var b strings.Builder
	b.WriteString("📊 BTC Agent — Bản tin chiến lược\n")
	b.WriteString(s.GeneratedAt.UTC().Format("02/01 15:04 UTC") + "\n")
	b.WriteString("───────────────────\n")

	b.WriteString("I. KẾT LUẬN\n")
	b.WriteString(actionConclusionVI(s.Analysis, s.Plan) + "\n")
	b.WriteString("Blocker chính: " + dominantBlockerVI(s.Analysis, s.Plan) + "\n")
	b.WriteString("Mode: " + vietnamesePermission(s.Analysis.ActionPermission) + " | Plan: " + vietnamesePlanState(s.Plan.State) + "\n")
	b.WriteString("───────────────────\n")

	b.WriteString("II. PHÂN TÍCH KỸ THUẬT BTC\n")
	b.WriteString(fmt.Sprintf("Giá BTC: %.0f USDT | Regime: %s | Trend score: %.1f/100\n", s.Analysis.BTCPrice, vietnameseRegime(s.Analysis.MarketRegime), s.Analysis.TrendScore))
	b.WriteString(fmt.Sprintf("Bias: tuần=%s, ngày=%s, 4H=%s\n", vietnameseBias(s.Analysis.WeeklyBias), vietnameseBias(s.Analysis.DailyBias), vietnameseBias(s.Analysis.FourHourBias)))
	b.WriteString(fmt.Sprintf("Flow: %s %.2f — %s\n", s.Analysis.Flow.Bias, s.Analysis.Flow.Score, flowDetailVI(s.Analysis)))
	b.WriteString(fmt.Sprintf("Rủi ro: tổng=%s | falling knife=%s | FOMO=%s\n", vietnameseRisk(s.Analysis.RiskLevel), vietnameseRisk(s.Analysis.FallingKnifeRisk), vietnameseRisk(s.Analysis.FomoRisk)))
	b.WriteString("Vùng giá quan trọng:\n")
	writeZoneVI(&b, "Gom active", s.Analysis.AccumulationZone.Low, s.Analysis.AccumulationZone.High)
	writeZoneVI(&b, "Gom macro/stress", s.Analysis.MacroAccumulationZone.Low, s.Analysis.MacroAccumulationZone.High)
	writeZoneVI(&b, "Support chính", s.Analysis.PrimarySupportZone.Low, s.Analysis.PrimarySupportZone.High)
	writeZoneVI(&b, "Support sâu", s.Analysis.DeepSupportZone.Low, s.Analysis.DeepSupportZone.High)
	writeZoneVI(&b, "Kháng cự", s.Analysis.ResistanceZone.Low, s.Analysis.ResistanceZone.High)
	writeZoneVI(&b, "Invalidation", s.Analysis.InvalidationZone.Low, s.Analysis.InvalidationZone.High)
	b.WriteString("───────────────────\n")

	b.WriteString("III. KỊCH BẢN THỊ TRƯỜNG\n")
	b.WriteString(marketScenarioVI(s.Analysis))
	b.WriteString("Điều kiện BTC cần thấy:\n")
	for _, item := range btcTriggerChecklistVI(s.Analysis) {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("───────────────────\n")

	b.WriteString("IV. KẾ HOẠCH BOT\n")
	b.WriteString(fmt.Sprintf("Permission: %s | Plan: %s\n", vietnamesePermission(s.Analysis.ActionPermission), vietnamesePlanState(s.Plan.State)))
	if s.Analysis.PermissionReason != "" {
		b.WriteString("Lý do chính: " + s.Analysis.PermissionReason + "\n")
	}
	active := activeAssetsVI(s.Plan)
	if len(active) > 0 {
		b.WriteString("Coin đủ điều kiện ACTIVE_LIMIT:\n")
		for _, asset := range active {
			b.WriteString(assetPlanLineVI(asset) + "\n")
			for _, layer := range asset.Layers {
				b.WriteString(fmt.Sprintf("  Layer %d: entry %.4f × %.2f USDT | RR %.2f | invalid %.4f | target %.4f\n", layer.Index, layer.Price, layer.Notional, layer.RewardRisk, layer.Invalidation, layer.Target))
			}
		}
	} else {
		b.WriteString("Không có ACTIVE_LIMIT. Bot không đặt lệnh, không chase giá.\n")
	}
	unlock := BuildUnlockConditions(s.Analysis, s.Plan)
	if len(unlock) > 0 {
		b.WriteString("Điều kiện mở khóa:\n")
		for _, item := range unlock {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(s.Plan.Watchlist.Candidates) > 0 {
		b.WriteString("Watchlist theo MM/liquidity:\n")
		limit := len(s.Plan.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range s.Plan.Watchlist.Candidates[:limit] {
			b.WriteString(watchCandidateLineVI(c) + "\n")
		}
	} else if len(s.Plan.Assets) > 0 {
		b.WriteString("Coin bị chặn theo plan:\n")
		limit := len(s.Plan.Assets)
		if limit > 3 {
			limit = 3
		}
		for _, asset := range s.Plan.Assets[:limit] {
			b.WriteString(assetPlanLineVI(asset) + "\n")
		}
	}
	if s.ShadowProbe.Profile != "" {
		b.WriteString("Shadow ARMED_PROBE_LIGHT:\n")
		b.WriteString(fmt.Sprintf("- production=%s, research=%s | would_probe=%v | Shadow only — không đặt lệnh thật.\n", s.ShadowProbe.ProductionPermission, s.ShadowProbe.ResearchPermission, len(s.ShadowProbe.Candidates) > 0))
		if len(s.ShadowProbe.Candidates) > 0 {
			for _, c := range s.ShadowProbe.Candidates {
				b.WriteString(fmt.Sprintf("- would_probe: %s layer %d entry %.4f RR %.2f notional %.2f\n", c.Symbol, c.Layer, c.Entry, c.RewardRisk, c.Notional))
			}
		} else if len(s.ShadowProbe.Blockers) > 0 {
			b.WriteString("- blocker chính: " + strings.Join(limitStrings(s.ShadowProbe.Blockers, 3), "; ") + "\n")
		}
	}
	b.WriteString("───────────────────\n")

	b.WriteString("V. TIN TỨC / RESEARCH\n")
	b.WriteString(researchSummaryVI(s.ResearchSummary) + "\n")
	b.WriteString("Research chỉ là bối cảnh phụ, không override Agent 1/2 và không tự mở lệnh.\n")
	b.WriteString("───────────────────\n")

	b.WriteString("VI. TRẠNG THÁI THỰC THI\n")
	b.WriteString(fmt.Sprintf("Daily: %s | Reconcile: %s\n", okWarnVI(s.DailyOK), okWarnVI(s.ReconcileOK)))
	if s.SupervisorSet {
		b.WriteString(fmt.Sprintf("Supervisor: %s | Action: %s\n", s.Supervisor.Status, s.Supervisor.Action))
		if s.Supervisor.Managed != nil {
			m := s.Supervisor.Managed
			b.WriteString(fmt.Sprintf("Orders: desired=%d đặt=%d hủy=%d thay=%d chặn=%d\n", len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
			b.WriteString(fmt.Sprintf("Gates: data=%s | reconcile=%s | risk=%s\n", m.DataHealth.Status, m.ReconcileSafety.Status, m.RiskGovernor.Status))
		}
	}
	if len(s.Notes) > 0 {
		b.WriteString("Cảnh báo hệ thống: " + strings.Join(s.Notes, "; ") + "\n")
	}
	b.WriteString("\nAn toàn: không futures, không leverage, không market order. Chỉ spot limit BUY post-only only khi Agent 2 ACTIVE_LIMIT và safety gate sạch.\n")
	return strings.TrimSpace(b.String()) + "\n"
}

func BuildMissingData() string {
	var b strings.Builder
	b.WriteString("📊 BTC Agent — Bản tin chiến lược\n")
	b.WriteString(time.Now().UTC().Format("02/01 15:04 UTC") + "\n")
	b.WriteString("───────────────────\n")
	b.WriteString("Không đủ dữ liệu phân tích mới. Giữ nguyên trạng thái an toàn, không đặt lệnh.\n")
	b.WriteString("Research-only / system-only: không override Agent 1/2.\n")
	return strings.TrimSpace(b.String()) + "\n"
}

func CompactPlan(plan agent2.Plan) map[string]any {
	assets := []map[string]any{}
	for _, a := range plan.Assets {
		layers := []map[string]any{}
		for _, l := range a.Layers {
			layers = append(layers, map[string]any{"index": l.Index, "price": l.Price, "notional": l.Notional, "invalidation": l.Invalidation, "target": l.Target, "reward_risk": l.RewardRisk, "reason": l.Reason})
		}
		assets = append(assets, map[string]any{
			"symbol": a.Symbol, "state": a.State, "reason": a.Reason,
			"rotation_rank": a.RotationRank, "rotation_score": a.RotationScore,
			"asset_flow_bias": a.AssetFlowBias, "asset_flow_score": a.AssetFlowScore,
			"mm_case": a.MMCase, "mm_score": a.MMScore, "mm_reasons": a.MMReasons, "mm_missing": a.MMMissing,
			"liquidity_grade": a.LiquidityQuality.Grade, "liquidity_score": a.LiquidityQuality.Score, "liquidity_reasons": a.LiquidityQuality.Reasons,
			"hard_blockers": a.HardBlockers, "soft_blockers": a.SoftBlockers, "next_trigger": a.NextTrigger,
			"reward_risk": a.RewardRisk, "reward_risk_detail": a.RewardRiskDetail,
			"zone_width_pct": a.ZoneWidthPct, "discount_gap_pct": a.DiscountGapPct, "zone_quality": a.ZoneQuality, "layers": layers,
		})
	}
	watch := []map[string]any{}
	limit := len(plan.Watchlist.Candidates)
	if limit > 5 {
		limit = 5
	}
	for _, c := range plan.Watchlist.Candidates[:limit] {
		watch = append(watch, map[string]any{
			"symbol": c.Symbol, "state": c.State, "readiness_score": c.ReadinessScore, "tier": c.Tier,
			"actionable": c.Actionable, "missing": c.Missing, "next_trigger": c.NextTrigger,
			"mm_case": c.MMCase, "mm_score": c.MMScore, "mm_missing": c.MMMissing, "mm_reasons": c.MMReasons,
			"liquidity_grade": c.LiquidityQuality.Grade, "liquidity_score": c.LiquidityQuality.Score, "liquidity_reasons": c.LiquidityQuality.Reasons,
			"entry_checklist": c.EntryChecklist,
			"zone_width_pct":  c.ZoneWidthPct, "discount_gap": c.DiscountGap, "zone_quality": c.ZoneQuality, "reward_risk": c.RewardRisk,
		})
	}
	return map[string]any{"state": plan.State, "summary": plan.Summary, "assets": assets, "watchlist": watch}
}

func dominantBlockerVI(analysis agent1.MarketAnalysis, plan agent2.Plan) string {
	if analysis.ActionPermission == agent1.NoTrade || analysis.ActionPermission == agent1.Watch {
		return fmt.Sprintf("BTC permission %s; WATCH/NO_TRADE không tạo order", analysis.ActionPermission)
	}
	if plan.State != agent2.StateActiveLimit {
		for _, c := range plan.Watchlist.Candidates {
			if len(c.Missing) > 0 {
				return fmt.Sprintf("%s thiếu %s", c.Symbol, c.Missing[0])
			}
			if c.NextTrigger != "" {
				return fmt.Sprintf("%s chờ trigger: %s", c.Symbol, c.NextTrigger)
			}
		}
		for _, a := range plan.Assets {
			if a.Reason != "" {
				return fmt.Sprintf("%s: %s", a.Symbol, a.Reason)
			}
		}
		return "chưa có coin đủ ACTIVE_LIMIT"
	}
	return "ACTIVE_LIMIT chỉ được thực thi nếu safety gate sạch"
}

func marketScenarioVI(analysis agent1.MarketAnalysis) string {
	base := firstNonEmptyScheduler(analysis.ScenarioMain, "Bảo toàn vốn; chỉ quan sát cho tới khi BTC có reclaim/flow rõ.")
	unlock := firstNonEmptyScheduler(analysis.ScenarioBullish, "BTC cần thoát WATCH, reclaim support/kháng cự gần, flow chuyển ACCUMULATION/BEAR_TRAP và volume bán cạn.")
	invalid := firstNonEmptyScheduler(analysis.ScenarioBearish, "Mất invalidation/support với volume bán tăng thì giữ NO_TRADE, không bắt dao rơi.")
	if analysis.ActionPermission == agent1.Watch || analysis.MarketRegime == "DOWNTREND" {
		base = "Bảo toàn vốn là chính: BTC còn WATCH/DOWNTREND nên bot không săn entry, chỉ ghi nhận vùng value. " + base
	}
	return fmt.Sprintf("Kịch bản chính: %s\nKịch bản mở khóa: %s\nKịch bản vô hiệu: %s\n", base, unlock, invalid)
}

func btcTriggerChecklistVI(analysis agent1.MarketAnalysis) []string {
	items := []string{}
	if analysis.TrendScore < 45 {
		items = append(items, fmt.Sprintf("Trend score cần tăng %.1f điểm để lên ARMED.", 45-analysis.TrendScore))
	} else if analysis.TrendScore < 60 {
		items = append(items, fmt.Sprintf("Trend score %.1f đủ gần ARMED nhưng chưa đủ ALLOWED 60.", analysis.TrendScore))
	}
	if analysis.MarketRegime != "ACCUMULATION" && analysis.MarketRegime != "WEAK_UPTREND" && analysis.MarketRegime != "RANGE" && analysis.MarketRegime != "RANGING" {
		items = append(items, "Regime cần chuyển sang ACCUMULATION/WEAK_UPTREND/RANGE.")
	}
	if analysis.Flow.Daily.Diagnostics.NextBullTrigger != "" {
		items = append(items, "Flow cần "+analysis.Flow.Daily.Diagnostics.NextBullTrigger+".")
	} else if analysis.Flow.Score < 0.25 {
		items = append(items, fmt.Sprintf("Flow score cần >=0.25; hiện %.2f.", analysis.Flow.Score))
	}
	if analysis.InvalidationZone.Low > 0 || analysis.InvalidationZone.High > 0 {
		items = append(items, fmt.Sprintf("Invalidation cần giữ %.0f–%.0f; mất vùng này thì đứng ngoài.", analysis.InvalidationZone.Low, analysis.InvalidationZone.High))
	}
	if len(items) == 0 {
		items = append(items, "BTC đã gần đủ điều kiện; còn cần coin đạt MM/liquidity/discount/RR.")
	}
	return limitStrings(items, 4)
}

func watchCandidateLineVI(c agent2.WatchCandidate) string {
	mmReason := firstNonEmptyScheduler(firstSchedulerString(c.MMMissing), firstSchedulerString(c.MMReasons), "chưa có footprint rõ")
	liqReason := firstNonEmptyScheduler(firstSchedulerString(c.LiquidityQuality.Reasons), "liquidity chưa có lý do chi tiết")
	trigger := firstNonEmptyScheduler(c.NextTrigger, "chờ trigger rõ")
	return fmt.Sprintf("- %s: readiness %.0f%% tier=%s | MM=%s %.0f/100 (%s) | Liq=%s %.0f/100 (%s) | Discount %.2f%% | RR %.2f | trigger: %s", c.Symbol, c.ReadinessScore*100, c.Tier, emptyMMCase(c.MMCase), c.MMScore, mmReason, emptyScheduler(c.LiquidityQuality.Grade, "n/a"), c.LiquidityQuality.Score, liqReason, c.DiscountGap*100, c.RewardRisk, trigger)
}

func assetPlanLineVI(a agent2.AssetPlan) string {
	blocker := firstNonEmptyScheduler(a.Reason, firstSchedulerString(a.HardBlockers), firstSchedulerString(a.SoftBlockers), "chưa đủ điều kiện")
	liqReason := firstSchedulerString(a.LiquidityQuality.Reasons)
	if liqReason == "" {
		liqReason = "liquidity chưa có blocker"
	}
	trigger := firstNonEmptyScheduler(a.NextTrigger, "chờ trigger rõ")
	return fmt.Sprintf("- %s [%s]: MM=%s %.0f/100 | Liq=%s %.0f/100 (%s) | Discount %.2f%% | RR %.2f | thiếu: %s | trigger: %s", a.Symbol, a.State, emptyMMCase(a.MMCase), a.MMScore, emptyScheduler(a.LiquidityQuality.Grade, "n/a"), a.LiquidityQuality.Score, liqReason, a.DiscountGapPct*100, a.RewardRisk, blocker, trigger)
}

func researchSummaryVI(summary string) string {
	trimmed := strings.TrimSpace(summary)
	if trimmed == "" {
		return "Không có research mới đủ quan trọng; bỏ qua nếu không trùng deterministic signal."
	}
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	if len(trimmed) > 180 {
		trimmed = trimmed[:177] + "..."
	}
	return trimmed
}

func firstSchedulerString(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func firstNonEmptyScheduler(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emptyMMCase(c agent2.MMCase) string {
	if c == "" {
		return "NO_DATA"
	}
	return string(c)
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

func flowDetailVI(analysis agent1.MarketAnalysis) string {
	parts := []string{}
	for _, c := range analysis.Flow.Daily.Components {
		if !c.Pass {
			continue
		}
		if c.Bull > 0 {
			parts = append(parts, fmt.Sprintf("%s +%.2f", c.Name, c.Bull))
		} else if c.Bear > 0 {
			parts = append(parts, fmt.Sprintf("%s -%.2f", c.Name, c.Bear))
		}
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 && analysis.Flow.Daily.Diagnostics.NextBullTrigger != "" {
		return analysis.Flow.Daily.Diagnostics.NextBullTrigger
	}
	if analysis.Flow.Daily.Diagnostics.NeedBullScore > 0 {
		parts = append(parts, fmt.Sprintf("thiếu %.2f bull score", analysis.Flow.Daily.Diagnostics.NeedBullScore))
	}
	return strings.Join(parts, "; ")
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

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
