package telegramreport

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/research"
	"btc-agent/internal/textsafe"
	"btc-agent/internal/usertext"
)

type LiveReadinessView struct {
	GeneratedAt                   time.Time
	Mode                          string
	AutoLiveEnv                   bool
	OperatorHalted                bool
	CredentialEnvPresent          map[string]bool
	PlanState                     agent2.State
	Proof                         liveguard.Proof
	OpenLiveOrders                int
	LivePositions                 int
	AutoLiveBlockers              []string
	LiveEnabled                   bool
	RealTradingEnabled            bool
	AutoExecute                   bool
	LiveAutoMode                  bool
	LiveAutoMaxNotional           float64
	RequireManualConfirm          bool
	ProofOnly                     bool
	OrderManagementEnabled        bool
	MaxAutoLayersPerAsset         int
	MaxOpenLiveOrdersPerAsset     int
	MaxOpenLiveOrdersTotal        int
	MaxLiveNotionalPerOrderUSDT   float64
	MaxLiveNotionalPerAssetUSDT   float64
	MaxLiveNotionalTotalUSDT      float64
	CancelIfPlanNotActive         bool
	CancelIfPriceAboveDiscountPct float64
	ReplaceIfPriceDriftPct        float64
	CancelStaleAfterMinutes       int
	DataHealth                    liveguard.DataHealthResult
	ReconcileSafety               liveguard.ReconcileSafetyResult
	RiskGovernor                  liveguard.RiskGovernorResult
	ManagedCoinSummaries          []liveguard.ManagedCoinSummary
}

func DailyHumanText(analysis agent1.MarketAnalysis, plan agent2.Plan) string {
	var b strings.Builder

	// ── Header ───────────────────────────────────────────────────────────────
	b.WriteString("📊 BTC Agent — Phân tích ngày\n")
	b.WriteString(fmt.Sprintf("🕐 %s\n", analysis.Timestamp.Format("02/01 15:04 UTC")))
	b.WriteString(separatorLine())

	// ── I. BTC & Thị trường ───────────────────────────────────────────────
	b.WriteString("I. BTC & THỊ TRƯỜNG\n")
	b.WriteString(fmt.Sprintf("Giá: $%.0f  |  Chế độ thị trường: %s\n", analysis.BTCPrice, humanRegime(analysis.MarketRegime)))
	b.WriteString(fmt.Sprintf("Điểm xu hướng: %.1f/100  |  Tham lam/sợ hãi: %s (%d)  |  Rủi ro: %s\n",
		analysis.TrendScore,
		analysis.FearGreed.Classification,
		analysis.FearGreed.Value,
		humanRiskEmoji(analysis.RiskLevel)))
	b.WriteString(fmt.Sprintf("Thiên hướng tuần/ngày/4H: %s/%s/%s  |  Dòng tiền: %s %.2f\n",
		shortBias(analysis.WeeklyBias),
		shortBias(analysis.DailyBias),
		shortBias(analysis.FourHourBias),
		analysis.Flow.Bias, analysis.Flow.Score))
	b.WriteString(separatorLine())

	// ── II. Vùng giá & Kịch bản ───────────────────────────────────────────
	b.WriteString("II. VÙNG GIÁ & KỊCH BẢN\n")
	if analysis.AccumulationZone.Low > 0 {
		b.WriteString(fmt.Sprintf("🟢 Gom: $%.0f–%.0f  |", analysis.AccumulationZone.Low, analysis.AccumulationZone.High))
	}
	if analysis.PrimarySupportZone.Low > 0 {
		b.WriteString(fmt.Sprintf("  🔵 Hỗ trợ: $%.0f–%.0f\n", analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High))
	} else {
		b.WriteString("\n")
	}
	if analysis.InvalidationZone.Low > 0 {
		b.WriteString(fmt.Sprintf("❌ Vô hiệu: $%.0f–%.0f  |", analysis.InvalidationZone.Low, analysis.InvalidationZone.High))
	}
	if analysis.ResistanceZone.Low > 0 {
		b.WriteString(fmt.Sprintf("  🔴 Kháng cự: $%.0f–%.0f\n", analysis.ResistanceZone.Low, analysis.ResistanceZone.High))
	} else {
		b.WriteString("\n")
	}
	if analysis.ScenarioMain != "" {
		b.WriteString(fmt.Sprintf("📌 %s\n", shortReason(analysis.ScenarioMain)))
	}
	if analysis.ScenarioBullish != "" {
		b.WriteString(fmt.Sprintf("🐂 Mở khóa: %s\n", shortReason(analysis.ScenarioBullish)))
	}
	if analysis.ScenarioBearish != "" {
		b.WriteString(fmt.Sprintf("🐻 Vô hiệu: %s\n", shortReason(analysis.ScenarioBearish)))
	}
	b.WriteString(separatorLine())

	// ── III. Kế hoạch giao dịch (Agent 2) ──────────────────────────────────
	b.WriteString("III. KẾ HOẠCH GIAO DỊCH\n")
	b.WriteString(fmt.Sprintf("Cổng BTC: %s  |  Kế hoạch: %s\n", ExplainPermission(analysis.ActionPermission), humanPlanStateEmoji(plan.State)))

	activeAssets := []agent2.AssetPlan{}
	for _, a := range plan.Assets {
		if a.State == agent2.StateActiveLimit {
			activeAssets = append(activeAssets, a)
		}
	}
	if len(activeAssets) > 0 {
		b.WriteString("🟩 ĐỦ ĐIỀU KIỆN ĐẶT LỆNH — Bot tự đặt lệnh giới hạn nếu các khóa an toàn đều đạt:\n")
		for _, a := range activeAssets {
			b.WriteString(fmt.Sprintf("  %s | RR=%.1f | rank #%d | MM=%s %.0f | Liq=%s %.0f\n",
				a.Symbol, a.RewardRisk, a.RotationRank,
				emptyMMCaseText(a.MMCase), a.MMScore,
				empty(a.LiquidityQuality.Grade, "n/a"), a.LiquidityQuality.Score))
			for _, l := range a.Layers {
				b.WriteString(fmt.Sprintf("    Tầng %d: $%.2f × %.0f USDT\n", l.Index, l.Price, l.Notional))
			}
		}
	}

	if len(plan.Watchlist.Candidates) > 0 {
		b.WriteString("👀 Danh sách theo dõi:\n")
		for _, c := range firstCandidates(plan.Watchlist.Candidates, 3) {
			b.WriteString(fmt.Sprintf("  %s %.0f%% | MM=%s %.0f | Liq=%s %.0f | gap %.1f%% RR %.2f",
				c.Symbol, c.ReadinessScore*100,
				emptyMMCaseText(c.MMCase), c.MMScore,
				empty(c.LiquidityQuality.Grade, "n/a"), c.LiquidityQuality.Score,
				c.DiscountGap*100, c.RewardRisk))
			if c.NextTrigger != "" {
				b.WriteString(fmt.Sprintf(" | điều kiện kích hoạt: %s", c.NextTrigger))
			}
			b.WriteString("\n")
			if len(c.Missing) > 0 {
				b.WriteString(fmt.Sprintf("    Thiếu: %s\n", humanList(c.Missing, 2)))
			} else if len(c.MMMissing) > 0 {
				b.WriteString(fmt.Sprintf("    Dấu vết tạo lập thiếu: %s\n", humanList(c.MMMissing, 2)))
			}
		}
	}

	for _, a := range plan.Assets {
		if a.State == agent2.StateWatch || a.State == agent2.StateArmed {
			b.WriteString(fmt.Sprintf("  %s [%s] rank #%d | MM=%s %.0f | Liq=%s %.0f\n",
				a.Symbol, a.State, a.RotationRank,
				emptyMMCaseText(a.MMCase), a.MMScore,
				empty(a.LiquidityQuality.Grade, "n/a"), a.LiquidityQuality.Score))
		}
	}
	b.WriteString(separatorLine())

	// ── IV. Hành động & Safety ───────────────────────────────────────────
	b.WriteString("IV. HÀNH ĐỘNG\n")
	b.WriteString(humanActionConclusion(analysis.ActionPermission, plan.State, len(activeAssets)))
	b.WriteString("⚠️ Chỉ mua giao ngay bằng lệnh giới hạn tạo thanh khoản. Không hợp đồng tương lai, không đòn bẩy, không lệnh thị trường.\n")

	return trimTelegram(b.String())
}

func separatorLine() string {
	return "───────────────────\n"
}

func shortBias(bias string) string {
	switch strings.ToUpper(bias) {
	case "BULLISH":
		return "🟢"
	case "BEARISH":
		return "🔴"
	case "NEUTRAL":
		return "⚪"
	case "ACCUMULATION":
		return "🟢ACC"
	case "DISTRIBUTION":
		return "🔴DIST"
	case "BEAR_TRAP":
		return "🟡TRAP"
	case "BULL_TRAP":
		return "🟡BULL_T"
	default:
		if bias == "" {
			return "—"
		}
		return bias
	}
}

func humanRegime(regime string) string {
	switch regime {
	case "UPTREND":
		return "🟢 UPTREND"
	case "DOWNTREND":
		return "🔴 DOWNTREND"
	case "RANGING":
		return "⚪ RANGING"
	case "PANIC_SELLING":
		return "🚨 PANIC SELLING"
	case "RECOVERY":
		return "🟡 RECOVERY"
	default:
		return regime
	}
}

func humanRiskEmoji(r agent1.Risk) string {
	switch r {
	case agent1.Low:
		return "🟢 LOW"
	case agent1.Medium:
		return "🟡 MEDIUM"
	case agent1.High:
		return "🔴 HIGH"
	default:
		return string(r)
	}
}

func humanPlanStateEmoji(state agent2.State) string {
	switch state {
	case agent2.StateActiveLimit:
		return "🟩 ĐỦ ĐIỀU KIỆN ĐẶT LỆNH — đã có tầng lệnh giới hạn hợp lệ"
	case agent2.StateArmed:
		return "🟡 CHUẨN BỊ — gần đủ điều kiện, chờ kích hoạt"
	case agent2.StateWatch:
		return "👀 THEO DÕI — chưa đặt lệnh"
	case agent2.StateNoTrade:
		return "🚫 KHÔNG GIAO DỊCH"
	default:
		return string(state)
	}
}

func readinessBar(score float64) string {
	filled := int(score * 5)
	if filled > 5 {
		filled = 5
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 5-filled)
	return "[" + bar + "]"
}

func shortReason(reason string) string {
	if len(reason) > 80 {
		return reason[:77] + "..."
	}
	return reason
}

func humanActionConclusion(perm agent1.Permission, state agent2.State, activeCount int) string {
	switch {
	case perm == agent1.NoTrade:
		return "🚫 KHÔNG giao dịch. BTC chưa cho phép. Giữ USDT.\n"
	case perm == agent1.Watch && state == agent2.StateWatch:
		return "👀 QUAN SÁT. Chưa có thế vào lệnh. Theo dõi vùng hỗ trợ, chờ dòng tiền xác nhận.\n"
	case perm == agent1.Armed:
		return "🟡 CHUẨN BỊ. BTC gần đủ điều kiện. Theo dõi chặt điều kiện kích hoạt để chuyển sang trạng thái được phép đặt lệnh.\n"
	case perm == agent1.Allowed && activeCount > 0:
		return fmt.Sprintf("✅ CÓ %d COIN ĐỦ ĐIỀU KIỆN. Bot tự đặt lệnh giới hạn nếu kiểm tra an toàn sạch và không có lý do chặn.\n", activeCount)
	case perm == agent1.Allowed && activeCount == 0:
		return "🟢 BTC đã cho phép tìm cơ hội, nhưng chưa coin nào đủ thế vào lệnh. Theo dõi danh sách.\n"
	default:
		return "👀 Tiếp tục theo dõi. Không đặt lệnh thủ công.\n"
	}
}

func LiveReadinessHumanText(r LiveReadinessView) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Kiểm tra sẵn sàng giao dịch thật\n\n")

	autoBlockers := len(r.AutoLiveBlockers)
	if autoBlockers == 0 {
		b.WriteString("✅ Tự động giao dịch thật: KHÔNG có lý do chặn. Bot có thể đặt lệnh khi kế hoạch đạt trạng thái được phép đặt lệnh.\n")
	} else {
		b.WriteString(fmt.Sprintf("🚫 Tự động giao dịch thật: %d lý do đang chặn.\n", autoBlockers))
	}
	b.WriteString(fmt.Sprintf("Kế hoạch: %s  |  Lệnh đang mở: %d  |  Vị thế: %d\n\n",
		r.PlanState, r.OpenLiveOrders, r.LivePositions))

	b.WriteString("1) Tài khoản & kết nối\n")
	b.WriteString(fmt.Sprintf("- Biến môi trường OKX: %s\n", credentialSummary(r.CredentialEnvPresent)))
	if r.Proof.Account.Enabled {
		b.WriteString(fmt.Sprintf("- USDT khả dụng: %.2f  |  Tối thiểu: %.2f  |  Xác thực: %s  |  Số dư: %s\n",
			r.Proof.Account.FreeUSDT, r.Proof.Account.MinRequiredUSDT,
			yesNo(r.Proof.Account.AuthOK), yesNo(r.Proof.Account.BalanceOK)))
		if r.Proof.Account.Error != "" {
			b.WriteString(fmt.Sprintf("- Lỗi: %s\n", r.Proof.Account.Error))
		}
	}

	b.WriteString("\n2) Cấu hình giao dịch thật\n")
	b.WriteString(fmt.Sprintf("- Chế độ: %s  |  Khóa vận hành: %s\n", empty(r.Mode, "chưa đặt"), haltText(r.OperatorHalted)))
	b.WriteString(fmt.Sprintf("- Biến cho phép tự động: %s  |  Tự động đặt lệnh: %v  |  Yêu cầu xác nhận tay: %v\n",
		enabledText(r.AutoLiveEnv), r.AutoExecute, r.RequireManualConfirm))
	b.WriteString(fmt.Sprintf("- Giới hạn live-auto tùy chọn: %s (%.2f USDT)  |  Chỉ kiểm tra, không đặt lệnh: %v\n",
		enabledText(r.LiveAutoMode), r.LiveAutoMaxNotional, r.ProofOnly))
	b.WriteString(fmt.Sprintf("- Bộ quản lý lệnh: bật=%v  |  %d tầng/coin  |  %d lệnh/coin  |  %d lệnh tổng\n",
		r.OrderManagementEnabled, r.MaxAutoLayersPerAsset, r.MaxOpenLiveOrdersPerAsset, r.MaxOpenLiveOrdersTotal))
	b.WriteString(fmt.Sprintf("- Vốn: %.0f USDT/lệnh  |  %.0f USDT/coin  |  %.0f USDT tổng\n",
		r.MaxLiveNotionalPerOrderUSDT, r.MaxLiveNotionalPerAssetUSDT, r.MaxLiveNotionalTotalUSDT))

	if r.DataHealth.Status != "" {
		b.WriteString(fmt.Sprintf("\n3) Sức khỏe hệ thống\n- Dữ liệu: %s\n", r.DataHealth.Status))
		if r.ReconcileSafety.Status != "" {
			b.WriteString(fmt.Sprintf("- Đối soát lệnh: %s\n", r.ReconcileSafety.Status))
		}
		if r.RiskGovernor.Status != "" {
			b.WriteString(fmt.Sprintf("- Bộ kiểm soát rủi ro: %s\n", r.RiskGovernor.Status))
		}
	}

	if len(r.AutoLiveBlockers) > 0 {
		b.WriteString("\n❌ Lý do chặn:\n")
		for _, reason := range r.AutoLiveBlockers {
			b.WriteString("  • " + ExplainBlocker(reason) + "\n")
		}
	}

	if len(r.ManagedCoinSummaries) > 0 && (r.Proof.Candidate.Symbol == "" || r.PlanState != agent2.StateActiveLimit) {
		b.WriteString("\n4) Vì sao chưa tự đặt lệnh\n")
		shown := 0
		for _, coin := range r.ManagedCoinSummaries {
			if shown >= 3 || coin.DesiredLayers > 0 || coin.Placed > 0 || coin.Kept > 0 {
				continue
			}
			reasons := coin.WhyNoOrder
			if len(reasons) > 0 {
				b.WriteString(fmt.Sprintf("- %s: %s — %s\n", coin.Symbol, coin.State, explainReasons(reasons, 2)))
			} else {
				b.WriteString(fmt.Sprintf("- %s: %s — chưa có ACTIVE_LIMIT/layer hợp lệ.\n", coin.Symbol, coin.State))
			}
			if coin.NextTrigger != "" {
				b.WriteString("  Điều kiện tiếp theo: " + coin.NextTrigger + "\n")
			}
			shown++
		}
	}

	if r.Proof.Candidate.Symbol != "" {
		b.WriteString(fmt.Sprintf("\n5) Lệnh dự kiến qua kiểm tra\n- %s %s giá giới hạn %.8f  |  giá trị %.2f USDT\n",
			r.Proof.Candidate.Side, r.Proof.Candidate.Symbol,
			r.Proof.Candidate.Price, r.Proof.Candidate.Notional))
	}

	return trimTelegram(b.String())
}

func LiveProofHumanText(proof liveguard.Proof) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Kiểm tra an toàn lệnh thật\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainProofStatus(proof.Status)))
	b.WriteString("Lệnh thật: KHÔNG đặt lệnh từ lần kiểm tra này.\n")
	if proof.Account.Enabled {
		b.WriteString(fmt.Sprintf("Tài khoản OKX: xác thực=%s, số dư=%s, USDT khả dụng=%.2f, tối thiểu=%.2f.\n", yesNo(proof.Account.AuthOK), yesNo(proof.Account.BalanceOK), proof.Account.FreeUSDT, proof.Account.MinRequiredUSDT))
	}
	if proof.Candidate.Symbol != "" {
		b.WriteString(fmt.Sprintf("Lệnh dự kiến: %s %s giá giới hạn %.8f, giá trị %.2f USDT, live auto=%v.\n", proof.Candidate.Side, proof.Candidate.Symbol, proof.Candidate.Price, proof.Candidate.Notional, proof.Candidate.LiveAuto))
	}
	if len(proof.Reasons) > 0 {
		b.WriteString("Lý do/điều kiện còn thiếu:\n")
		for _, reason := range proof.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	b.WriteString("Hành động: chỉ theo dõi cho tới khi kiểm tra báo sẵn sàng và người vận hành chủ động xác nhận.\n")
	return trimTelegram(b.String())
}

func LiveOrderHumanText(result liveguard.ExecutionResult, auto bool) string {
	var b strings.Builder
	if auto {
		b.WriteString("🤖 BTC Agent — Lệnh thật tự động\n\n")
	} else {
		b.WriteString("🤖 BTC Agent — Lệnh thật đặt tay\n\n")
	}
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainOrderStatus(result.Status)))
	if result.Order.Submitted {
		b.WriteString("Lệnh thật: ĐÃ gửi lệnh lên OKX. Cần đối soát để theo dõi trạng thái/khớp lệnh.\n")
		b.WriteString(fmt.Sprintf("Mã lệnh: %s, mã phía bot %s.\n", result.Order.OrderID, result.Order.ClientOrderID))
	} else {
		b.WriteString("Lệnh thật: KHÔNG đặt lệnh.\n")
	}
	if result.Candidate.Symbol != "" {
		b.WriteString(fmt.Sprintf("Lệnh dự kiến: %s %s giá giới hạn %.8f, giá trị %.2f USDT, live auto=%v.\n", result.Candidate.Side, result.Candidate.Symbol, result.Candidate.Price, result.Candidate.Notional, result.Candidate.LiveAuto))
	}
	if len(result.Reasons) > 0 {
		b.WriteString("Lý do:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	return trimTelegram(b.String())
}

func LiveOrderManagementHumanText(result liveguard.ManagedCycleResult) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Quản lý lệnh thật\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", result.Summary))
	if result.DryRun {
		b.WriteString("Chế độ: CHẠY THỬ — chỉ mô phỏng, không gửi/hủy lệnh OKX.\n")
	}
	b.WriteString(fmt.Sprintf("Kế hoạch: %s — %s\n", result.PlanState, ExplainPlanState(result.PlanState)))
	b.WriteString(fmt.Sprintf("Đã giữ: %d. Đã hủy: %d. Đã thay thế: %d. Đã đặt mới: %d. Bị chặn: %d.\n", len(result.Kept), len(result.Canceled), len(result.Replaced), len(result.Placed), len(result.Blocked)))
	if len(result.Desired) == 0 {
		b.WriteString("Không có tầng lệnh đủ điều kiện ở chu kỳ này.\n")
	}
	writeDecisions := func(title string, items []liveguard.ManagedOrderDecision) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n" + title + ":\n")
		for _, item := range items {
			b.WriteString("- " + managementDecisionText(item) + "\n")
		}
	}
	writeDecisions("Giữ lệnh", result.Kept)
	writeDecisions("Hủy lệnh", result.Canceled)
	writeDecisions("Thay thế lệnh", result.Replaced)
	writeDecisions("Đặt mới", result.Placed)
	writeDecisions("Bị chặn", result.Blocked)
	if len(result.PerCoin) > 0 {
		b.WriteString("\nTheo từng coin:\n")
		for _, coin := range result.PerCoin {
			b.WriteString(fmt.Sprintf("\n%s\n", coin.Symbol))
			b.WriteString(fmt.Sprintf("- Trạng thái: %s — %s\n", coin.State, ExplainPlanState(coin.State)))
			b.WriteString(fmt.Sprintf("- Lệnh đang mở: %d\n", coin.OpenOrders))
			b.WriteString(fmt.Sprintf("- Tầng lệnh mong muốn: %d\n", coin.DesiredLayers))
			b.WriteString(fmt.Sprintf("- Vốn đang chờ: %.2f USDT\n", coin.PendingNotional))
			if len(coin.Actions) == 0 {
				b.WriteString("- Hành động: không làm gì\n")
			} else {
				b.WriteString("- Hành động: " + coinActionSummary(coin.Actions) + "\n")
			}
			if len(coin.Reasons) > 0 {
				b.WriteString("- Lý do: " + explainReasons(coin.Reasons, 3) + "\n")
			} else if len(coin.WhyNoOrder) > 0 {
				b.WriteString("- Vì sao chưa đặt: " + explainReasons(coin.WhyNoOrder, 3) + "\n")
			} else if coin.DesiredLayers == 0 && coin.OpenOrders == 0 {
				b.WriteString("- Lý do: chưa có ACTIVE_LIMIT/layer hợp lệ cho coin này.\n")
			}
			if coin.NextTrigger != "" {
				b.WriteString("- Điều kiện kích hoạt tiếp theo: " + coin.NextTrigger + "\n")
			}
		}
	}
	if len(result.Reasons) > 0 {
		b.WriteString("\nLý do hệ thống:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	b.WriteString("\nAn toàn: chỉ mua giao ngay bằng lệnh giới hạn tạo thanh khoản; không hợp đồng tương lai, không đòn bẩy, không lệnh thị trường.\n")
	return trimTelegram(b.String())
}

func LiveSupervisorHumanText(result liveguard.SupervisorResult) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Giám sát giao dịch thật\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", result.Summary))
	if result.AutoHalted {
		b.WriteString("🚨 Khóa vận hành: ĐÃ BẬT tự động sau lỗi lặp lại. Cần người vận hành kiểm tra.\n")
	}
	if len(result.Reasons) > 0 {
		b.WriteString("Lý do:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	if result.Managed != nil {
		m := result.Managed
		b.WriteString(fmt.Sprintf("\nChu kỳ: %s  |  mong muốn=%d đặt=%d hủy=%d thay=%d chặn=%d\n",
			m.Status, len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
		if m.DataHealth.Status != "" {
			b.WriteString(fmt.Sprintf("Dữ liệu: %s  |  Đối soát: %s  |  Rủi ro: %s\n",
				m.DataHealth.Status, m.ReconcileSafety.Status, m.RiskGovernor.Status))
		}
		// Per-coin detail khi không đặt lệnh
		if len(m.Desired) == 0 && len(m.PerCoin) > 0 {
			b.WriteString("\nTại sao chưa đặt lệnh:\n")
			for _, coin := range m.PerCoin {
				b.WriteString(fmt.Sprintf("  %s [%s]", coin.Symbol, coin.State))
				if len(coin.WhyNoOrder) > 0 {
					b.WriteString(": " + explainReasons(coin.WhyNoOrder, 2))
				} else if coin.DesiredLayers == 0 {
					b.WriteString(": chưa có trạng thái được phép đặt lệnh/tầng lệnh")
				}
				if coin.NextTrigger != "" {
					b.WriteString(" → " + coin.NextTrigger)
				}
				b.WriteString("\n")
			}
		}
		// Placed orders detail
		if len(m.Placed) > 0 {
			b.WriteString("\nĐã đặt:\n")
			for _, item := range m.Placed {
				b.WriteString("  ✅ " + managementDecisionText(item) + "\n")
			}
		}
		if len(m.Canceled) > 0 {
			b.WriteString("\nĐã hủy:\n")
			for _, item := range m.Canceled {
				b.WriteString("  ❌ " + managementDecisionText(item) + "\n")
			}
		}
	}
	b.WriteString("\n⚠️ Chỉ mua giao ngay bằng lệnh giới hạn tạo thanh khoản; không hợp đồng tương lai, không đòn bẩy, không lệnh thị trường.\n")
	return trimTelegram(b.String())
}

func ResearchBriefHumanText(result research.BriefResult) string {
	var b strings.Builder
	b.WriteString("🔍 BTC Agent — Tóm tắt tin tức chiến lược\n")
	b.WriteString(fmt.Sprintf("🕐 %s\n", result.GeneratedAt.Format("02/01 15:04 UTC")))
	b.WriteString(separatorLine())

	// #13: clearly state when research is disabled
	if result.Status == research.BriefWarn && len(result.Warnings) > 0 {
		for _, w := range result.Warnings {
			if w == "research disabled" {
				b.WriteString("⚫ Thu thập tin tức đang tắt (research.enabled=false trong cấu hình).\n")
				b.WriteString("Không có tin tức được thu thập. Bật research.enabled=true để nhận bản tóm tắt.\n")
				b.WriteString("Tin tức chỉ để tham khảo: không đặt lệnh, không ghi đè Agent 1/2.\n")
				return trimTelegram(b.String())
			}
		}
	}

	warnItems := []research.ResearchItem{}
	infoItems := []research.ResearchItem{}
	allTags := []string{}
	for _, item := range result.Items {
		allTags = append(allTags, item.Tags...)
		if item.Risk == research.RiskWarn {
			warnItems = append(warnItems, item)
		} else {
			infoItems = append(infoItems, item)
		}
	}
	allTags = uniqueTiny(allTags)

	stance := "NEUTRAL / WATCH"
	if len(warnItems) > 0 {
		stance = "MIXED → ưu tiên phòng thủ"
	} else if len(infoItems) >= 3 {
		stance = "NEUTRAL-RISK ON nhẹ"
	}

	b.WriteString("I. KẾT LUẬN\n")
	b.WriteString(fmt.Sprintf("Lập trường: %s. Tin tức là bối cảnh, không đổi quyền giao dịch.\n", stance))
	b.WriteString("Bot vẫn bám Agent 1/2: chỉ vào lệnh nếu có trạng thái được phép đặt lệnh và các khóa an toàn đều sạch.\n")
	b.WriteString(separatorLine())

	b.WriteString("II. LUẬN ĐIỂM\n")
	if len(allTags) > 0 {
		b.WriteString("Chủ đề nổi bật: " + strings.Join(firstStrings(allTags, 6), ", ") + ".\n")
	}
	if len(infoItems) > 0 {
		b.WriteString("Tin nền nghiêng về theo dõi dòng tiền và tâm lý, chưa đủ làm tín hiệu mua độc lập.\n")
	}
	if len(result.Items) == 0 {
		b.WriteString("Chưa có tin mới đủ dùng để nâng cấp nhận định.\n")
	}
	b.WriteString(separatorLine())

	b.WriteString("III. RỦI RO\n")
	if len(warnItems) == 0 {
		b.WriteString("Chưa có cảnh báo lớn từ lớp tin tức; vẫn cần tránh mua vội và chờ vùng chiết khấu.\n")
	} else {
		for _, item := range firstResearchItems(warnItems, 3) {
			b.WriteString("- " + compactNewsTitle(item) + "\n")
		}
	}
	b.WriteString(separatorLine())

	b.WriteString("IV. CƠ HỘI\n")
	if len(infoItems) == 0 {
		b.WriteString("Chưa có chất xúc tác rõ. Giữ danh sách theo dõi, chờ giá và dòng tiền xác nhận.\n")
	} else {
		for _, item := range firstResearchItems(infoItems, 3) {
			b.WriteString("- " + compactNewsTitle(item) + "\n")
		}
	}
	b.WriteString(separatorLine())

	b.WriteString("V. KẾ HOẠCH BOT\n")
	b.WriteString("Hành động ưu tiên: THEO DÕI / GIỮ TIỀN. Không mua đuổi theo tin.\n")
	b.WriteString("Chỉ cân nhắc mua giao ngay bằng lệnh giới hạn tạo thanh khoản nếu Agent 2 tạo trạng thái được phép đặt lệnh và bộ kiểm soát rủi ro đạt.\n")
	b.WriteString(fmt.Sprintf("Nguồn xử lý: %d tin / %d nguồn. Link URL không gửi vào Telegram.\n", len(result.Items), result.SourcesChecked))
	if len(result.Warnings) > 0 {
		b.WriteString("Cảnh báo thu thập: " + result.Warnings[0] + "\n")
	}
	b.WriteString("Tin tức chỉ để tham khảo: không đặt lệnh, không ghi đè Agent 1/2.\n")

	return trimTelegram(b.String())
}

func firstResearchItems(items []research.ResearchItem, limit int) []research.ResearchItem {
	if len(items) < limit {
		limit = len(items)
	}
	return items[:limit]
}

func compactNewsTitle(item research.ResearchItem) string {
	tags := ""
	if len(item.Tags) > 0 {
		tags = " [" + strings.Join(firstStrings(item.Tags, 4), ",") + "]"
	}
	return shortReason(item.Title) + tags
}

func firstStrings(items []string, limit int) []string {
	if len(items) < limit {
		limit = len(items)
	}
	return items[:limit]
}

func uniqueTiny(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func managementDecisionText(item liveguard.ManagedOrderDecision) string {
	symbol := firstNonEmptyString(item.Symbol, item.Desired.Symbol, item.Order.Symbol, live.InternalSymbol(item.Order.InstID))
	layer := firstNonZeroInt(item.LayerIndex, item.Desired.LayerIndex, item.Order.LayerIndex)
	out := fmt.Sprintf("%s tầng %d: %s", symbol, layer, ExplainBlocker(item.Reason))
	price := item.Desired.Price
	notional := item.Desired.Notional
	if price <= 0 {
		price = item.Order.Price
	}
	if notional <= 0 {
		notional = item.Order.Notional
	}
	if price > 0 {
		out += fmt.Sprintf(" | giá giới hạn %.8f", price)
	}
	if notional > 0 {
		out += fmt.Sprintf(", %.2f USDT", notional)
	}
	if item.Desired.AllocationTier != "" {
		out += fmt.Sprintf(" | hạng=%s điểm=%.1f", item.Desired.AllocationTier, item.Desired.AllocationScore)
	}
	if item.Desired.AllocationReason != "" {
		out += " | phân bổ: " + item.Desired.AllocationReason
	}
	if item.Error != "" {
		out += " | lỗi: " + item.Error
	}
	return out
}

func coinActionSummary(actions []liveguard.ManagedOrderDecision) string {
	parts := []string{}
	for _, item := range actions {
		layer := firstNonZeroInt(item.LayerIndex, item.Desired.LayerIndex, item.Order.LayerIndex)
		parts = append(parts, fmt.Sprintf("%s tầng %d", humanManagementAction(item.Action), layer))
		if len(parts) >= 4 {
			break
		}
	}
	if len(actions) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d hành động khác", len(actions)-len(parts)))
	}
	return strings.Join(parts, ", ")
}

func humanManagementAction(action string) string {
	switch action {
	case "keep":
		return "giữ"
	case "cancel", "would_cancel":
		return "hủy"
	case "replace":
		return "thay thế"
	case "place", "would_place":
		return "đặt"
	case "block":
		return "chặn"
	default:
		return action
	}
}

func explainReasons(reasons []string, limit int) string {
	items := []string{}
	for _, reason := range reasons {
		items = append(items, ExplainBlocker(reason))
		if len(items) >= limit {
			break
		}
	}
	if len(reasons) > limit {
		items = append(items, fmt.Sprintf("+%d lý do khác", len(reasons)-limit))
	}
	return strings.Join(items, "; ")
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonZeroInt(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func ReconcileHumanText(result liveguard.ReconcileResult, ledger liveguard.LiveLedgerReport) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Đối soát lệnh thật\n\n")
	b.WriteString("Lệnh mới: KHÔNG đặt lệnh. Đây chỉ là kiểm tra trạng thái lệnh đã có.\n")
	b.WriteString(fmt.Sprintf("Đã kiểm tra %d lệnh: cập nhật %d, cần kiểm tra tay %d.\n", result.Checked, result.Updated, result.Unknown))
	if len(result.Orders) == 0 {
		b.WriteString("Không có lệnh thật đang mở trong dữ liệu nội bộ.\n")
	}
	for _, o := range result.Orders {
		b.WriteString(fmt.Sprintf("- %s: trạng thái %s, đã khớp %.8f, giá trung bình %.8f.\n", o.InstID, ExplainExchangeStatus(o.Status), o.FilledQuantity, o.AvgPrice))
	}
	b.WriteString(positionSummary(ledger))
	return trimTelegram(b.String())
}

func PositionHumanText(report liveguard.LiveLedgerReport) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Vị thế giao dịch thật\n\n")
	b.WriteString("Lệnh mới: KHÔNG đặt lệnh. Đây chỉ là đọc sổ theo dõi nội bộ.\n")
	b.WriteString(positionSummary(report))
	return trimTelegram(b.String())
}

func ExplainPlanState(state agent2.State) string {
	switch state {
	case agent2.StateNoTrade:
		return "Không giao dịch; điều kiện thị trường chưa đạt."
	case agent2.StateWatch:
		return "Chỉ theo dõi, chưa được phép tạo lệnh."
	case agent2.StateArmed:
		return "Gần đủ điều kiện, chờ điều kiện kích hoạt rõ hơn."
	case agent2.StateActiveLimit:
		return "Đã có lệnh giới hạn hợp lệ từ bộ quyết định cố định."
	default:
		return "Trạng thái chưa rõ; giữ an toàn và không đặt lệnh."
	}
}

func ExplainProofStatus(status string) string {
	switch status {
	case liveguard.ReadyForManualLiveProofOrder:
		return "Kiểm tra đã sẵn sàng cho lệnh thật vốn nhỏ, nhưng vẫn cần người vận hành xác nhận và khóa an toàn phù hợp."
	case liveguard.NotReadyNoDeterministicOrder:
		return "Chưa có lệnh hợp lệ vì Agent 2 chưa tạo tầng lệnh ở trạng thái được phép đặt."
	case liveguard.NotReadyBalance:
		return "Chưa sẵn sàng vì kiểm tra số dư OKX chưa đạt."
	case liveguard.NotReadyFilters:
		return "Chưa sẵn sàng vì preflight OKX/tick size/lot size chưa đạt."
	case liveguard.NotReadyConfig:
		return "Chưa sẵn sàng vì cấu hình hoặc biến môi trường giao dịch thật chưa đủ."
	default:
		if status == "" {
			return "Chưa có kết quả kiểm tra an toàn."
		}
		return status
	}
}

func ExplainPermission(p agent1.Permission) string {
	switch p {
	case agent1.Allowed:
		return "ĐƯỢC PHÉP — BTC đủ điều kiện để Agent 2 tìm thế vào lệnh."
	case agent1.Armed:
		return "CHUẨN BỊ — gần đủ điều kiện, chưa nên đặt lệnh."
	case agent1.Watch:
		return "THEO DÕI — chưa được phép đặt lệnh."
	case agent1.NoTrade:
		return "KHÔNG GIAO DỊCH."
	default:
		return string(p)
	}
}

func ExplainRisk(level agent1.Risk) string {
	switch level {
	case agent1.Low:
		return "THẤP — rủi ro thấp"
	case agent1.Medium:
		return "VỪA — rủi ro vừa, cần chờ xác nhận"
	case agent1.High:
		return "CAO — rủi ro cao, ưu tiên bảo toàn vốn"
	default:
		return string(level)
	}
}

func ExplainOrderStatus(status string) string {
	switch status {
	case liveguard.LiveOrderSubmitted:
		return "ĐÃ GỬI LỆNH"
	case liveguard.LiveOrderBlocked:
		return "BỊ CHẶN AN TOÀN"
	case liveguard.LiveOrderRejected:
		return "OKX TỪ CHỐI / LỖI GỬI LỆNH"
	default:
		return status
	}
}

func ExplainExchangeStatus(status string) string {
	switch status {
	case live.StatusLiveOpen:
		return "đang mở/chờ khớp"
	case live.StatusPartiallyFilled:
		return "khớp một phần"
	case live.StatusFilled:
		return "đã khớp xong"
	case live.StatusCanceled:
		return "đã hủy"
	case live.StatusRejected:
		return "bị từ chối"
	case live.StatusUnknownNeedsManualCheck:
		return "không rõ, cần kiểm tra tay"
	default:
		return status
	}
}

func ExplainBlocker(reason string) string {
	s := strings.TrimSpace(reason)
	switch {
	case s == "BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution":
		return "Tự động giao dịch thật chưa bật bằng biến môi trường; đây là khóa an toàn."
	case s == "operator halt active":
		return "Khóa vận hành đang bật; bot bị khóa đặt lệnh thật."
	case s == "open live order exists" || strings.Contains(s, "open live order exists"):
		return "Đang có lệnh thật mở; phải đối soát/khớp/hủy trước khi đặt lệnh mới."
	case s == "confirm phrase required":
		return "Thiếu câu xác nhận manual bắt buộc."
	case s == "live.auto_execute=false":
		return "Tự động đặt lệnh trong cấu hình đang tắt."
	case s == "live.require_manual_confirm=true":
		return "Config đang yêu cầu xác nhận tay, nên auto không chạy."
	case s == "live.require_manual_confirm=false":
		return "Config không yêu cầu xác nhận tay; manual live bị chặn."
	case s == "live.enabled=false":
		return "Giao dịch thật trong cấu hình đang tắt."
	case s == "live.proof_only=true":
		return "Cấu hình chỉ cho kiểm tra an toàn, không cho đặt lệnh thật."
	case s == "execution.real_trading_enabled=false":
		return "Real trading trong config đang tắt."
	case s == "account check not pass":
		return "Kiểm tra tài khoản OKX hoặc số dư chưa đạt."
	case strings.HasPrefix(s, "proof not ready:"):
		return "Kiểm tra an toàn chưa sẵn sàng: " + ExplainProofStatus(strings.TrimSpace(strings.TrimPrefix(s, "proof not ready:")))
	case s == "ladder total notional must be positive":
		return "Chưa có tổng giá trị rải lệnh vì chưa có tầng lệnh hợp lệ."
	case s == "ladder total notional above max":
		return "Tổng giá trị rải lệnh vượt giới hạn an toàn."
	case s == "no ladder candidates":
		return "Chưa có tầng lệnh giới hạn hợp lệ để rải lệnh."
	case s == "open live order limit reached":
		return "Số lệnh thật đang mở đã đạt giới hạn an toàn."
	case s == "order still matches active accumulation layer":
		return "Lệnh vẫn khớp vùng gom/tầng hiện tại, tiếp tục giữ."
	case s == "missing active accumulation layer order":
		return "Tầng vùng gom đang thiếu lệnh thật nên bot đặt mới."
	case s == "order no longer matches active asset/layer":
		return "Lệnh không còn khớp coin/tầng đang được phép đặt nên bot hủy."
	case s == "order no longer matches current desired layer":
		return "Lệnh không còn khớp giá/vùng mua mới nên bot hủy hoặc thay thế."
	case s == "plan no longer ACTIVE_LIMIT":
		return "Plan không còn ACTIVE_LIMIT nên bot hủy lệnh chờ."
	case s == "per-asset open order limit reached":
		return "Coin này đã đạt số lệnh mở tối đa."
	case s == "total open order limit reached":
		return "Tổng số lệnh mở đã đạt giới hạn an toàn."
	case s == "per-asset live notional cap reached":
		return "Coin này đã đạt giới hạn vốn giao dịch thật."
	case s == "total live notional cap reached":
		return "Tổng vốn giao dịch thật đã đạt giới hạn an toàn."
	case s == "operator halt active":
		return "Khóa vận hành đang bật; bot bị khóa quản lý lệnh thật."
	case s == "order placer/canceler unavailable":
		return "Không tạo được OKX client để đặt/hủy lệnh."
	case s == "preflight not pass":
		return "Kiểm tra trước khi gửi lên OKX chưa đạt bước giá/khối lượng tối thiểu/giá trị tối thiểu."
	case s == "order placer unavailable":
		return "Không tạo được OKX client để gửi lệnh."
	case s == "no deterministic ACTIVE_LIMIT layer available":
		return "Agent 2 chưa tạo tầng lệnh được phép đặt nên không có lệnh hợp lệ."
	case s == "BTC permission chưa ALLOWED":
		return "BTC chưa đủ điều kiện thị trường để gom altcoin."
	case strings.Contains(s, "required live credential env is not set"):
		return "Thiếu env credential OKX; kiểm tra file env riêng."
	default:
		return s
	}
}

func firstCandidates(in []agent2.WatchCandidate, limit int) []agent2.WatchCandidate {
	if len(in) < limit {
		limit = len(in)
	}
	return in[:limit]
}

func humanList(items []string, limit int) string {
	out := []string{}
	for _, item := range items {
		out = append(out, ExplainBlocker(item))
		if len(out) >= limit {
			break
		}
	}
	if len(items) > limit {
		out = append(out, fmt.Sprintf("+%d điều kiện khác", len(items)-limit))
	}
	return strings.Join(out, "; ")
}

func humanFlow(bias any) string {
	s := fmt.Sprint(bias)
	switch s {
	case "ACCUMULATION":
		return "có dấu hiệu tích lũy, nhưng vẫn cần các khóa khác xác nhận."
	case "BEAR_TRAP":
		return "có dấu hiệu bear trap/reclaim."
	case "DISTRIBUTION", "BULL_TRAP":
		return "cảnh báo phân phối/bull trap, không nên đuổi giá."
	case "NEUTRAL":
		return "flow chưa rõ, chưa đủ làm tín hiệu vào lệnh."
	default:
		return "flow cần được xác nhận thêm."
	}
}

func credentialSummary(m map[string]bool) string {
	if len(m) == 0 {
		return "chưa kiểm tra"
	}
	missing := []string{}
	for _, key := range []string{"OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"} {
		if ok, exists := m[key]; exists && !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return "đủ"
	}
	return "thiếu " + strings.Join(missing, ", ")
}

func positionSummary(report liveguard.LiveLedgerReport) string {
	var b strings.Builder
	if len(report.Positions) == 0 {
		b.WriteString("Không có vị thế giao dịch thật trong sổ nội bộ.\n")
	} else {
		b.WriteString("Vị thế giao dịch thật:\n")
		for _, p := range report.Positions {
			b.WriteString(fmt.Sprintf("- %s: qty %.8f, entry %.8f, cost %.2f USDT.\n", p.Symbol, p.Quantity, p.AvgEntryPrice, p.CostBasis))
		}
	}
	if len(report.ManualCheckRequired) > 0 {
		b.WriteString("Cần kiểm tra tay:\n")
		for _, item := range report.ManualCheckRequired {
			b.WriteString("- " + item + "\n")
		}
	}
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "OK"
	}
	return "chưa đạt"
}

func enabledText(v bool) string {
	if v {
		return "đã bật"
	}
	return "chưa bật"
}

func haltText(v bool) string {
	if v {
		return "ĐANG BẬT — bot bị khóa đặt lệnh thật"
	}
	return "đang tắt — giao dịch thật có thể chạy nếu các khóa khác cũng đạt"
}

func empty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func emptyMMCaseText(c agent2.MMCase) string {
	if c == "" {
		return "NO_DATA"
	}
	return string(c)
}

func trimTelegram(s string) string {
	return textsafe.TrimTelegram(usertext.TelegramVietnamese(s), 3500)
}
