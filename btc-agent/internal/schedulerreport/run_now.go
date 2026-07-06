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
	b.WriteString(actionConclusionVI(s.Analysis, s.Plan))
	b.WriteString("\n")

	b.WriteString("II. PHÂN TÍCH KỸ THUẬT BTC\n")
	b.WriteString(fmt.Sprintf("Giá BTC: %.0f USDT | Regime: %s | Trend score: %.1f/100\n", s.Analysis.BTCPrice, vietnameseRegime(s.Analysis.MarketRegime), s.Analysis.TrendScore))
	b.WriteString(fmt.Sprintf("Bias: tuần=%s, ngày=%s, 4H=%s\n", vietnameseBias(s.Analysis.WeeklyBias), vietnameseBias(s.Analysis.DailyBias), vietnameseBias(s.Analysis.FourHourBias)))
	b.WriteString(fmt.Sprintf("Flow: %s %.2f — %s\n", s.Analysis.Flow.Bias, s.Analysis.Flow.Score, vietnameseFlowNote(fmt.Sprint(s.Analysis.Flow.Bias))))
	b.WriteString(fmt.Sprintf("Rủi ro: tổng=%s | falling knife=%s | FOMO=%s\n", vietnameseRisk(s.Analysis.RiskLevel), vietnameseRisk(s.Analysis.FallingKnifeRisk), vietnameseRisk(s.Analysis.FomoRisk)))
	b.WriteString("\nVùng giá quan trọng:\n")
	writeZoneVI(&b, "Gom", s.Analysis.AccumulationZone.Low, s.Analysis.AccumulationZone.High)
	writeZoneVI(&b, "Support chính", s.Analysis.PrimarySupportZone.Low, s.Analysis.PrimarySupportZone.High)
	writeZoneVI(&b, "Support sâu", s.Analysis.DeepSupportZone.Low, s.Analysis.DeepSupportZone.High)
	writeZoneVI(&b, "Kháng cự", s.Analysis.ResistanceZone.Low, s.Analysis.ResistanceZone.High)
	writeZoneVI(&b, "Invalidation", s.Analysis.InvalidationZone.Low, s.Analysis.InvalidationZone.High)
	b.WriteString("───────────────────\n")

	b.WriteString("III. KỊCH BẢN THỊ TRƯỜNG\n")
	if s.Analysis.ScenarioMain != "" {
		b.WriteString("Chính: " + s.Analysis.ScenarioMain + "\n")
	}
	if s.Analysis.ScenarioBullish != "" {
		b.WriteString("Tốt: " + s.Analysis.ScenarioBullish + "\n")
	}
	if s.Analysis.ScenarioBearish != "" {
		b.WriteString("Xấu: " + s.Analysis.ScenarioBearish + "\n")
	}
	b.WriteString("───────────────────\n")

	b.WriteString("IV. KẾ HOẠCH BOT\n")
	b.WriteString(fmt.Sprintf("Permission: %s | Plan: %s\n", vietnamesePermission(s.Analysis.ActionPermission), vietnamesePlanState(s.Plan.State)))
	active := activeAssetsVI(s.Plan)
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
	if len(s.Plan.Watchlist.Candidates) > 0 {
		b.WriteString("Watchlist gần đạt:\n")
		limit := len(s.Plan.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range s.Plan.Watchlist.Candidates[:limit] {
			b.WriteString(fmt.Sprintf("- %s: readiness %.0f%% | chờ: %s\n", c.Symbol, c.ReadinessScore*100, emptyScheduler(c.NextTrigger, "thêm xác nhận")))
		}
	}
	b.WriteString("───────────────────\n")

	b.WriteString("V. TIN TỨC / RESEARCH\n")
	b.WriteString(emptyScheduler(s.ResearchSummary, "Không có research mới") + "\n")
	b.WriteString("Tin tức chỉ là bối cảnh, không dùng để tự đặt lệnh.\n")
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
	b.WriteString("\nAn toàn: không futures, không leverage, không market order. Chỉ spot limit BUY post-only khi Agent 2 ACTIVE_LIMIT và safety gate sạch.\n")
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
