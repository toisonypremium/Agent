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
	CanaryMode                    bool
	CanaryMaxNotional             float64
	RequireManualConfirm          bool
	ProofOnly                     bool
	AutoLadderEnabled             bool
	MaxAutoLayers                 int
	MaxOpenLiveOrders             int
	AutoLadderMaxNotional         float64
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
	LadderProof                   liveguard.LadderProof
	DataHealth                    liveguard.DataHealthResult
	ReconcileSafety               liveguard.ReconcileSafetyResult
	RiskGovernor                  liveguard.RiskGovernorResult
}

func DailyHumanText(analysis agent1.MarketAnalysis, plan agent2.Plan) string {
	var b strings.Builder

	// ── Header ───────────────────────────────────────────────────────────────
	b.WriteString("📊 BTC Agent — Phân tích ngày\n")
	b.WriteString(fmt.Sprintf("🕐 %s\n", analysis.Timestamp.Format("02/01 15:04 UTC")))
	b.WriteString(separatorLine())

	// ── I. Tổng quan thị trường ───────────────────────────────────────────
	b.WriteString("I. THỊ TRƯỜNG BTC\n")
	b.WriteString(fmt.Sprintf("Giá: $%.0f  |  Regime: %s\n", analysis.BTCPrice, humanRegime(analysis.MarketRegime)))
	b.WriteString(fmt.Sprintf("Trend: %.1f/100  |  F&G: %s (%d)\n",
		analysis.TrendScore,
		analysis.FearGreed.Classification,
		analysis.FearGreed.Value))
	b.WriteString(fmt.Sprintf("Bias: W=%s  D=%s  4H=%s\n",
		shortBias(analysis.WeeklyBias),
		shortBias(analysis.DailyBias),
		shortBias(analysis.FourHourBias)))
	b.WriteString(fmt.Sprintf("Flow: %s %.2f — %s\n",
		analysis.Flow.Bias, analysis.Flow.Score, humanFlow(analysis.Flow.Bias)))
	b.WriteString(separatorLine())

	// ── II. Rủi ro ────────────────────────────────────────────────────────
	b.WriteString("II. RỦI RO\n")
	b.WriteString(fmt.Sprintf("Tổng rủi ro: %s\n", humanRiskEmoji(analysis.RiskLevel)))
	b.WriteString(fmt.Sprintf("Falling knife: %s  |  FOMO: %s\n",
		humanRiskEmoji(analysis.FallingKnifeRisk),
		humanRiskEmoji(analysis.FomoRisk)))
	b.WriteString(separatorLine())

	// ── III. Vùng giá then chốt ───────────────────────────────────────────
	b.WriteString("III. VÙNG GIÁ\n")
	if analysis.AccumulationZone.Low > 0 {
		b.WriteString(fmt.Sprintf("🟢 Gom: $%.0f – $%.0f\n",
			analysis.AccumulationZone.Low, analysis.AccumulationZone.High))
	}
	if analysis.PrimarySupportZone.Low > 0 {
		b.WriteString(fmt.Sprintf("🔵 Support: $%.0f – $%.0f\n",
			analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High))
	}
	if analysis.DeepSupportZone.Low > 0 {
		b.WriteString(fmt.Sprintf("⚫ Deep: $%.0f – $%.0f\n",
			analysis.DeepSupportZone.Low, analysis.DeepSupportZone.High))
	}
	if analysis.ResistanceZone.Low > 0 {
		b.WriteString(fmt.Sprintf("🔴 Kháng cự: $%.0f – $%.0f\n",
			analysis.ResistanceZone.Low, analysis.ResistanceZone.High))
	}
	if analysis.InvalidationZone.Low > 0 {
		b.WriteString(fmt.Sprintf("❌ Invalidation: $%.0f – $%.0f\n",
			analysis.InvalidationZone.Low, analysis.InvalidationZone.High))
	}
	b.WriteString(separatorLine())

	// ── IV. Kịch bản ─────────────────────────────────────────────────────
	b.WriteString("IV. KỊCH BẢN\n")
	if analysis.ScenarioMain != "" {
		b.WriteString(fmt.Sprintf("📌 Chính: %s\n", analysis.ScenarioMain))
	}
	if analysis.ScenarioBullish != "" {
		b.WriteString(fmt.Sprintf("🐂 Bull: %s\n", analysis.ScenarioBullish))
	}
	if analysis.ScenarioBearish != "" {
		b.WriteString(fmt.Sprintf("🐻 Bear: %s\n", analysis.ScenarioBearish))
	}
	b.WriteString(separatorLine())

	// ── V. Kế hoạch giao dịch (Agent 2) ──────────────────────────────────
	b.WriteString("V. KẾ HOẠCH GIAO DỊCH\n")
	b.WriteString(fmt.Sprintf("Permission: %s\n", ExplainPermission(analysis.ActionPermission)))
	b.WriteString(fmt.Sprintf("Plan state: %s\n", humanPlanStateEmoji(plan.State)))

	// Active assets with layers
	activeAssets := []agent2.AssetPlan{}
	for _, a := range plan.Assets {
		if a.State == agent2.StateActiveLimit {
			activeAssets = append(activeAssets, a)
		}
	}
	if len(activeAssets) > 0 {
		b.WriteString("\n🟩 ACTIVE LIMIT — Đã có layer:\n")
		for _, a := range activeAssets {
			b.WriteString(fmt.Sprintf("  %s | RR=%.1f | rank #%d\n",
				a.Symbol, a.RewardRisk, a.RotationRank))
			for _, l := range a.Layers {
				b.WriteString(fmt.Sprintf("    Layer %d: $%.2f × %.0f USDT\n",
					l.Index, l.Price, l.Notional))
			}
			if a.Reason != "" {
				b.WriteString(fmt.Sprintf("    → %s\n", shortReason(a.Reason)))
			}
		}
	}

	// Watchlist
	if len(plan.Watchlist.Candidates) > 0 {
		b.WriteString("\n👀 WATCHLIST (sắp đủ điều kiện):\n")
		for _, c := range firstCandidates(plan.Watchlist.Candidates, 3) {
			bar := readinessBar(c.ReadinessScore)
			b.WriteString(fmt.Sprintf("  %s %s %.0f%%", c.Symbol, bar, c.ReadinessScore*100))
			if c.NextTrigger != "" {
				b.WriteString(fmt.Sprintf(" | chờ: %s", c.NextTrigger))
			}
			b.WriteString("\n")
			if len(c.Missing) > 0 {
				b.WriteString(fmt.Sprintf("    Thiếu: %s\n", humanList(c.Missing, 2)))
			}
		}
	}

	// Rotation ranking (non-active)
	watchOrArmed := []agent2.AssetPlan{}
	for _, a := range plan.Assets {
		if a.State == agent2.StateWatch || a.State == agent2.StateArmed {
			watchOrArmed = append(watchOrArmed, a)
		}
	}
	if len(watchOrArmed) > 0 {
		b.WriteString("\n⏳ THEO DÕI:\n")
		for _, a := range watchOrArmed {
			b.WriteString(fmt.Sprintf("  %s [%s] rank #%d score %.2f\n",
				a.Symbol, a.State, a.RotationRank, a.RotationScore))
		}
	}
	b.WriteString(separatorLine())

	// ── VI. Kết luận hành động ───────────────────────────────────────────
	b.WriteString("VI. HÀNH ĐỘNG\n")
	b.WriteString(humanActionConclusion(analysis.ActionPermission, plan.State, len(activeAssets)))
	b.WriteString("\n⚠️ Spot limit BUY post-only. Không futures/leverage/market.\n")

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
		return "🟩 ACTIVE_LIMIT — Đã có layer limit hợp lệ"
	case agent2.StateArmed:
		return "🟡 ARMED — Gần đủ điều kiện, chờ trigger"
	case agent2.StateWatch:
		return "👀 WATCH — Theo dõi, chưa đặt lệnh"
	case agent2.StateNoTrade:
		return "🚫 NO_TRADE — Không giao dịch"
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
		return "👀 QUAN SÁT. Chưa có setup. Theo dõi vùng support, chờ flow xác nhận.\n"
	case perm == agent1.Armed:
		return "🟡 CHUẨN BỊ. BTC gần đủ điều kiện. Theo dõi chặt trigger để chuyển ACTIVE_LIMIT.\n"
	case perm == agent1.Allowed && activeCount > 0:
		return fmt.Sprintf("✅ CÓ %d COIN ĐỦ ĐIỀU KIỆN. Bot tự đặt limit nếu proof sạch và không có blocker.\n", activeCount)
	case perm == agent1.Allowed && activeCount == 0:
		return "🟢 BTC ALLOWED nhưng chưa coin nào đủ setup. Theo dõi watchlist.\n"
	default:
		return "👀 Tiếp tục theo dõi. Không đặt lệnh thủ công.\n"
	}
}

func LiveReadinessHumanText(r LiveReadinessView) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Live readiness\n\n")
	ready := r.Proof.Status == liveguard.ReadyForManualLiveProofOrder
	if ready {
		b.WriteString("Kết luận: CÓ proof hợp lệ cho manual canary.\n")
	} else {
		b.WriteString("Kết luận: CHƯA SẴN SÀNG ĐẶT LỆNH.\n")
	}
	b.WriteString("Lệnh thật: KHÔNG đặt lệnh từ kiểm tra này.\n")
	b.WriteString(fmt.Sprintf("Lý do chính: %s\n\n", ExplainProofStatus(r.Proof.Status)))

	b.WriteString("1) Trạng thái bot\n")
	b.WriteString(fmt.Sprintf("- Mode: %s.\n", empty(r.Mode, "chưa đặt")))
	b.WriteString(fmt.Sprintf("- Plan hiện tại: %s — %s\n", r.PlanState, ExplainPlanState(r.PlanState)))
	b.WriteString(fmt.Sprintf("- Open live orders: %d. Live positions: %d.\n", r.OpenLiveOrders, r.LivePositions))
	if r.DataHealth.Status != "" {
		b.WriteString(fmt.Sprintf("- Data health: %s — %s\n", r.DataHealth.Status, r.DataHealth.Summary))
	}
	if r.ReconcileSafety.Status != "" {
		b.WriteString(fmt.Sprintf("- Reconcile safety: %s — %s\n", r.ReconcileSafety.Status, r.ReconcileSafety.Summary))
	}
	if r.RiskGovernor.Status != "" {
		b.WriteString(fmt.Sprintf("- Risk governor: %s — %s\n", r.RiskGovernor.Status, r.RiskGovernor.Summary))
	}

	b.WriteString("\n2) Tài khoản & kết nối\n")
	b.WriteString(fmt.Sprintf("- OKX env: %s.\n", credentialSummary(r.CredentialEnvPresent)))
	if r.Proof.Account.Enabled {
		b.WriteString(fmt.Sprintf("- USDT free: %.2f / yêu cầu tối thiểu %.2f. Auth: %s. Balance: %s.\n", r.Proof.Account.FreeUSDT, r.Proof.Account.MinRequiredUSDT, yesNo(r.Proof.Account.AuthOK), yesNo(r.Proof.Account.BalanceOK)))
		if r.Proof.Account.Error != "" {
			b.WriteString(fmt.Sprintf("- Lỗi account: %s\n", r.Proof.Account.Error))
		}
	}

	b.WriteString("\n3) Khóa an toàn\n")
	b.WriteString(fmt.Sprintf("- Operator halt: %s.\n", haltText(r.OperatorHalted)))
	b.WriteString(fmt.Sprintf("- Auto live env: %s. Auto execute config: %v.\n", enabledText(r.AutoLiveEnv), r.AutoExecute))
	b.WriteString(fmt.Sprintf("- Canary: %v, max %.2f USDT. Manual confirm: %v.\n", r.CanaryMode, r.CanaryMaxNotional, r.RequireManualConfirm))
	b.WriteString(fmt.Sprintf("- Legacy auto ladder: enabled=%v, tối đa %d layer/chu kỳ, open orders legacy tối đa %d, notional tối đa %.2f USDT.\n", r.AutoLadderEnabled, r.MaxAutoLayers, r.MaxOpenLiveOrders, r.AutoLadderMaxNotional))
	b.WriteString(fmt.Sprintf("- Managed order engine: enabled=%v, tối đa %d layer/coin, %d lệnh/coin, %d lệnh tổng.\n", r.OrderManagementEnabled, r.MaxAutoLayersPerAsset, r.MaxOpenLiveOrdersPerAsset, r.MaxOpenLiveOrdersTotal))
	b.WriteString(fmt.Sprintf("- Managed vốn: %.2f USDT/lệnh, %.2f USDT/coin, %.2f USDT tổng.\n", r.MaxLiveNotionalPerOrderUSDT, r.MaxLiveNotionalPerAssetUSDT, r.MaxLiveNotionalTotalUSDT))
	b.WriteString(fmt.Sprintf("- Managed hủy/thay: hủy khi plan inactive=%v, giá vượt discount %.2f%%, drift %.2f%%, stale %d phút.\n", r.CancelIfPlanNotActive, r.CancelIfPriceAboveDiscountPct*100, r.ReplaceIfPriceDriftPct*100, r.CancelStaleAfterMinutes))
	if r.CanaryMode && r.OrderManagementEnabled {
		b.WriteString("- Logic mở lệnh: hard safety vẫn khóa nguy hiểm; tín hiệu trade dùng risk sizing.\n")
		b.WriteString("- Opportunity allocation: vốn live đi theo setup score hiện tại, không chia cứng theo % portfolio.\n")
		b.WriteString("- Quality multiplier: A/B full, C giảm size, NO_SAMPLE/missing chỉ probe nhỏ, D bị chặn.\n")
	}
	if r.LadderProof.Status != "" {
		b.WriteString(fmt.Sprintf("- Ladder proof: %s, candidates=%d, total=%.2f USDT.\n", ExplainProofStatus(r.LadderProof.Status), len(r.LadderProof.Candidates), r.LadderProof.TotalNotional))
	}
	if len(r.AutoLiveBlockers) > 0 {
		b.WriteString("- Auto live đang bị chặn vì:\n")
		for _, reason := range r.AutoLiveBlockers {
			b.WriteString("  • " + ExplainBlocker(reason) + "\n")
		}
	}

	if r.Proof.Candidate.Symbol != "" {
		b.WriteString("\n4) Candidate nếu proof hợp lệ\n")
		b.WriteString(fmt.Sprintf("- %s %s limit %.8f, notional %.2f USDT, post-only=%v, canary=%v.\n", r.Proof.Candidate.Side, r.Proof.Candidate.Symbol, r.Proof.Candidate.Price, r.Proof.Candidate.Notional, r.Proof.Candidate.PostOnly, r.Proof.Candidate.Canary))
		if r.Proof.Preflight.Enabled {
			b.WriteString(fmt.Sprintf("- Preflight OKX: %s. Notional sau filter %.2f USDT.\n", yesNo(r.Proof.Preflight.Pass), r.Proof.Preflight.Notional))
		}
	}

	b.WriteString("\nHành động đề xuất: tiếp tục live-proof 24/7, chưa resume, chưa bật auto, chưa đặt lệnh.\n")
	return trimTelegram(b.String())
}

func LiveProofHumanText(proof liveguard.Proof) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Live proof\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainProofStatus(proof.Status)))
	b.WriteString("Lệnh thật: KHÔNG đặt lệnh từ proof này.\n")
	if proof.Account.Enabled {
		b.WriteString(fmt.Sprintf("OKX account: auth=%s, balance=%s, USDT free=%.2f, tối thiểu=%.2f.\n", yesNo(proof.Account.AuthOK), yesNo(proof.Account.BalanceOK), proof.Account.FreeUSDT, proof.Account.MinRequiredUSDT))
	}
	if proof.Candidate.Symbol != "" {
		b.WriteString(fmt.Sprintf("Candidate: %s %s limit %.8f, notional %.2f USDT, canary=%v.\n", proof.Candidate.Side, proof.Candidate.Symbol, proof.Candidate.Price, proof.Candidate.Notional, proof.Candidate.Canary))
	}
	if len(proof.Reasons) > 0 {
		b.WriteString("Lý do/điều kiện còn thiếu:\n")
		for _, reason := range proof.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	b.WriteString("Hành động: chỉ theo dõi cho tới khi proof READY và operator chủ động xác nhận.\n")
	return trimTelegram(b.String())
}

func LiveOrderHumanText(result liveguard.ExecutionResult, auto bool) string {
	var b strings.Builder
	if auto {
		b.WriteString("🤖 BTC Agent — Auto live order\n\n")
	} else {
		b.WriteString("🤖 BTC Agent — Manual live order\n\n")
	}
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainOrderStatus(result.Status)))
	if result.Order.Submitted {
		b.WriteString("Lệnh thật: ĐÃ gửi lệnh lên OKX. Cần reconcile để theo dõi trạng thái/fill.\n")
		b.WriteString(fmt.Sprintf("Order: %s, client id %s.\n", result.Order.OrderID, result.Order.ClientOrderID))
	} else {
		b.WriteString("Lệnh thật: KHÔNG đặt lệnh.\n")
	}
	if result.Candidate.Symbol != "" {
		b.WriteString(fmt.Sprintf("Candidate: %s %s limit %.8f, notional %.2f USDT, canary=%v.\n", result.Candidate.Side, result.Candidate.Symbol, result.Candidate.Price, result.Candidate.Notional, result.Candidate.Canary))
	}
	if len(result.Reasons) > 0 {
		b.WriteString("Lý do:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	return trimTelegram(b.String())
}

func LiveLadderOrderHumanText(result liveguard.LadderExecutionResult) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Auto ladder\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainOrderStatus(result.Status)))
	if result.Status == liveguard.LiveOrderSubmitted {
		b.WriteString(fmt.Sprintf("Lệnh thật: ĐÃ gửi %d lệnh limit lên OKX.\n", len(result.Orders)))
	} else {
		b.WriteString("Lệnh thật: KHÔNG đặt lệnh hoặc chỉ đặt một phần trước khi bị chặn/lỗi.\n")
	}
	b.WriteString(fmt.Sprintf("Tổng notional dự kiến: %.2f USDT.\n", result.TotalNotional))
	if len(result.Candidates) > 0 {
		b.WriteString("Danh sách layer:\n")
		for i, c := range result.Candidates {
			b.WriteString(fmt.Sprintf("%d) %s %s limit %.8f, notional %.2f USDT, post-only=%v, canary=%v.\n", i+1, c.Side, c.Symbol, c.Price, c.Notional, c.PostOnly, c.Canary))
		}
	}
	if len(result.Reasons) > 0 {
		b.WriteString("Lý do/cảnh báo:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	b.WriteString("Việc cần theo dõi: reconcile order, kiểm tra fill, giữ không futures/no leverage/no market order.\n")
	return trimTelegram(b.String())
}

func LiveOrderManagementHumanText(result liveguard.ManagedCycleResult) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Quản lý live orders\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", result.Summary))
	if result.DryRun {
		b.WriteString("Chế độ: DRY-RUN — chỉ mô phỏng, không gửi/hủy lệnh OKX.\n")
	}
	b.WriteString(fmt.Sprintf("Plan: %s — %s\n", result.PlanState, ExplainPlanState(result.PlanState)))
	b.WriteString(fmt.Sprintf("Đã giữ: %d. Đã hủy: %d. Đã thay thế: %d. Đã đặt mới: %d. Bị chặn: %d.\n", len(result.Kept), len(result.Canceled), len(result.Replaced), len(result.Placed), len(result.Blocked)))
	if len(result.Desired) == 0 {
		b.WriteString("Không có layer ACTIVE_LIMIT hợp lệ ở chu kỳ này.\n")
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
			b.WriteString(fmt.Sprintf("- State: %s — %s\n", coin.State, ExplainPlanState(coin.State)))
			b.WriteString(fmt.Sprintf("- Open orders: %d\n", coin.OpenOrders))
			b.WriteString(fmt.Sprintf("- Desired layers: %d\n", coin.DesiredLayers))
			b.WriteString(fmt.Sprintf("- Pending: %.2f USDT\n", coin.PendingNotional))
			if len(coin.Actions) == 0 {
				b.WriteString("- Action: không làm gì\n")
			} else {
				b.WriteString("- Action: " + coinActionSummary(coin.Actions) + "\n")
			}
			if len(coin.Reasons) > 0 {
				b.WriteString("- Lý do: " + explainReasons(coin.Reasons, 3) + "\n")
			} else if len(coin.WhyNoOrder) > 0 {
				b.WriteString("- Vì sao chưa đặt: " + explainReasons(coin.WhyNoOrder, 3) + "\n")
			} else if coin.DesiredLayers == 0 && coin.OpenOrders == 0 {
				b.WriteString("- Lý do: chưa có ACTIVE_LIMIT/layer hợp lệ cho coin này.\n")
			}
			if coin.NextTrigger != "" {
				b.WriteString("- Trigger tiếp theo: " + coin.NextTrigger + "\n")
			}
		}
	}
	if len(result.Reasons) > 0 {
		b.WriteString("\nLý do hệ thống:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	b.WriteString("\nAn toàn: chỉ spot limit BUY post-only, không futures, không leverage, không market order.\n")
	return trimTelegram(b.String())
}

func LiveSupervisorHumanText(result liveguard.SupervisorResult) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Live supervisor\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", result.Summary))
	b.WriteString(fmt.Sprintf("Action: %s. Consecutive errors: %d. Auto-halt: %v.\n", result.Action, result.ConsecutiveErrors, result.AutoHalted))
	if result.AutoHalted {
		b.WriteString("Operator halt: ĐÃ BẬT tự động sau lỗi lặp lại. Bot không tự hủy lệnh; cần operator kiểm tra.\n")
	}
	if len(result.Reasons) > 0 {
		b.WriteString("Lý do:\n")
		for _, reason := range result.Reasons {
			b.WriteString("- " + ExplainBlocker(reason) + "\n")
		}
	}
	if result.Managed != nil {
		m := result.Managed
		b.WriteString("\nManaged cycle:\n")
		b.WriteString(fmt.Sprintf("- Status: %s\n", m.Status))
		b.WriteString(fmt.Sprintf("- Desired: %d. Placed: %d. Canceled: %d. Replaced: %d. Blocked: %d.\n", len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
		if m.DataHealth.Status != "" {
			b.WriteString(fmt.Sprintf("- Data health: %s\n", m.DataHealth.Status))
		}
		if m.ReconcileSafety.Status != "" {
			b.WriteString(fmt.Sprintf("- Reconcile safety: %s\n", m.ReconcileSafety.Status))
		}
		if m.RiskGovernor.Status != "" {
			b.WriteString(fmt.Sprintf("- Risk governor: %s\n", m.RiskGovernor.Status))
		}
	}
	b.WriteString("\nAn toàn: chỉ spot limit BUY post-only, không futures, không leverage, không market order.\n")
	return trimTelegram(b.String())
}

func ResearchBriefHumanText(result research.BriefResult) string {
	var b strings.Builder
	b.WriteString("🔍 BTC Agent — Research Strategy Brief\n")
	b.WriteString(fmt.Sprintf("🕐 %s\n", result.GeneratedAt.Format("02/01 15:04 UTC")))
	b.WriteString(separatorLine())

	// #13: clearly state when research is disabled
	if result.Status == research.BriefWarn && len(result.Warnings) > 0 {
		for _, w := range result.Warnings {
			if w == "research disabled" {
				b.WriteString("⚫ Research đang tắt (research.enabled=false trong config).\n")
				b.WriteString("Không có tin tức được thu thập. Bật research.enabled=true để nhận brief.\n")
				b.WriteString("Research-only: không đặt lệnh, không override Agent 1/2.\n")
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
	b.WriteString(fmt.Sprintf("Stance: %s. Tin tức là context, không đổi quyền giao dịch.\n", stance))
	b.WriteString("Bot vẫn bám Agent 1/2: chỉ vào lệnh nếu có ACTIVE_LIMIT + safety gate sạch.\n")
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
		b.WriteString("Chưa có cảnh báo lớn từ news layer; vẫn cần tránh FOMO và chờ vùng discount.\n")
	} else {
		for _, item := range firstResearchItems(warnItems, 3) {
			b.WriteString("- " + compactNewsTitle(item) + "\n")
		}
	}
	b.WriteString(separatorLine())

	b.WriteString("IV. CƠ HỘI\n")
	if len(infoItems) == 0 {
		b.WriteString("Chưa có catalyst rõ. Giữ watchlist, chờ giá và flow xác nhận.\n")
	} else {
		for _, item := range firstResearchItems(infoItems, 3) {
			b.WriteString("- " + compactNewsTitle(item) + "\n")
		}
	}
	b.WriteString(separatorLine())

	b.WriteString("V. KẾ HOẠCH BOT\n")
	b.WriteString("Action bias: WATCH / HOLD CASH. Không chase theo tin.\n")
	b.WriteString("Chỉ cân nhắc spot limit BUY post-only nếu Agent 2 tạo ACTIVE_LIMIT và risk gate OK.\n")
	b.WriteString(fmt.Sprintf("Nguồn xử lý: %d tin / %d nguồn. Link URL không gửi vào Telegram.\n", len(result.Items), result.SourcesChecked))
	if len(result.Warnings) > 0 {
		b.WriteString("Cảnh báo thu thập: " + result.Warnings[0] + "\n")
	}
	b.WriteString("Research-only: không đặt lệnh, không override Agent 1/2.\n")

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
	out := fmt.Sprintf("%s layer %d: %s", symbol, layer, ExplainBlocker(item.Reason))
	price := item.Desired.Price
	notional := item.Desired.Notional
	if price <= 0 {
		price = item.Order.Price
	}
	if notional <= 0 {
		notional = item.Order.Notional
	}
	if price > 0 {
		out += fmt.Sprintf(" | limit %.8f", price)
	}
	if notional > 0 {
		out += fmt.Sprintf(", %.2f USDT", notional)
	}
	if item.Desired.AllocationTier != "" {
		out += fmt.Sprintf(" | tier=%s score=%.1f", item.Desired.AllocationTier, item.Desired.AllocationScore)
	}
	if item.Desired.AllocationReason != "" {
		out += " | allocation: " + item.Desired.AllocationReason
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
		parts = append(parts, fmt.Sprintf("%s layer %d", humanManagementAction(item.Action), layer))
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
	b.WriteString("🤖 BTC Agent — Reconcile live orders\n\n")
	b.WriteString("Lệnh mới: KHÔNG đặt lệnh. Đây chỉ là kiểm tra trạng thái lệnh đã có.\n")
	b.WriteString(fmt.Sprintf("Đã kiểm tra %d lệnh: cập nhật %d, cần kiểm tra tay %d.\n", result.Checked, result.Updated, result.Unknown))
	if len(result.Orders) == 0 {
		b.WriteString("Không có live order mở trong DB.\n")
	}
	for _, o := range result.Orders {
		b.WriteString(fmt.Sprintf("- %s: trạng thái %s, filled %.8f, avg %.8f.\n", o.InstID, ExplainExchangeStatus(o.Status), o.FilledQuantity, o.AvgPrice))
	}
	b.WriteString(positionSummary(ledger))
	return trimTelegram(b.String())
}

func PositionHumanText(report liveguard.LiveLedgerReport) string {
	var b strings.Builder
	b.WriteString("🤖 BTC Agent — Live positions\n\n")
	b.WriteString("Lệnh mới: KHÔNG đặt lệnh. Đây chỉ là đọc ledger nội bộ.\n")
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
		return "Gần đủ điều kiện, chờ trigger rõ hơn."
	case agent2.StateActiveLimit:
		return "Đã có lệnh limit hợp lệ từ engine deterministic."
	default:
		return "Trạng thái chưa rõ; giữ an toàn và không đặt lệnh."
	}
}

func ExplainProofStatus(status string) string {
	switch status {
	case liveguard.ReadyForManualLiveProofOrder:
		return "Proof đã sẵn sàng cho manual canary, nhưng vẫn cần operator xác nhận và khóa an toàn phù hợp."
	case liveguard.NotReadyNoDeterministicOrder:
		return "Chưa có lệnh hợp lệ vì Agent 2 chưa tạo layer ACTIVE_LIMIT."
	case liveguard.NotReadyBalance:
		return "Chưa sẵn sàng vì kiểm tra số dư OKX chưa đạt."
	case liveguard.NotReadyFilters:
		return "Chưa sẵn sàng vì preflight OKX/tick size/lot size chưa đạt."
	case liveguard.NotReadyConfig:
		return "Chưa sẵn sàng vì cấu hình hoặc env live chưa đủ."
	default:
		if status == "" {
			return "Chưa có proof."
		}
		return status
	}
}

func ExplainPermission(p agent1.Permission) string {
	switch p {
	case agent1.Allowed:
		return "ALLOWED — BTC đủ điều kiện để Agent 2 tìm setup."
	case agent1.Armed:
		return "ARMED — gần đủ điều kiện, chưa nên đặt lệnh."
	case agent1.Watch:
		return "WATCH — chỉ theo dõi, chưa được phép đặt lệnh."
	case agent1.NoTrade:
		return "NO_TRADE — không giao dịch."
	default:
		return string(p)
	}
}

func ExplainRisk(level agent1.Risk) string {
	switch level {
	case agent1.Low:
		return "LOW — rủi ro thấp"
	case agent1.Medium:
		return "MEDIUM — rủi ro vừa, cần chờ xác nhận"
	case agent1.High:
		return "HIGH — rủi ro cao, ưu tiên bảo toàn vốn"
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
		return "Auto live chưa bật bằng biến môi trường; đây là khóa an toàn."
	case s == "operator halt active":
		return "Operator halt đang bật; bot bị khóa đặt lệnh thật."
	case s == "open live order exists" || strings.Contains(s, "open live order exists"):
		return "Đang có live order mở; phải reconcile/fill/hủy trước khi đặt lệnh mới."
	case s == "confirm phrase required":
		return "Thiếu câu xác nhận manual bắt buộc."
	case s == "live.auto_execute=false":
		return "Auto execute trong config đang tắt."
	case s == "live.require_manual_confirm=true":
		return "Config đang yêu cầu xác nhận tay, nên auto không chạy."
	case s == "live.require_manual_confirm=false":
		return "Config không yêu cầu xác nhận tay; manual live bị chặn."
	case s == "live.enabled=false":
		return "Live mode trong config đang tắt."
	case s == "live.proof_only=true":
		return "Config chỉ cho proof, không cho đặt lệnh thật."
	case s == "execution.real_trading_enabled=false":
		return "Real trading trong config đang tắt."
	case s == "account check not pass":
		return "Kiểm tra tài khoản OKX hoặc số dư chưa đạt."
	case strings.HasPrefix(s, "proof not ready:"):
		return "Proof chưa sẵn sàng: " + ExplainProofStatus(strings.TrimSpace(strings.TrimPrefix(s, "proof not ready:")))
	case s == "ladder total notional must be positive":
		return "Chưa có tổng notional ladder vì chưa có layer hợp lệ."
	case s == "ladder total notional above max":
		return "Tổng notional ladder vượt giới hạn an toàn."
	case s == "no ladder candidates":
		return "Chưa có layer limit hợp lệ để rải lệnh."
	case s == "open live order limit reached":
		return "Số live order đang mở đã đạt giới hạn an toàn."
	case s == "order still matches active accumulation layer":
		return "Lệnh vẫn khớp vùng gom/layer hiện tại, tiếp tục giữ."
	case s == "missing active accumulation layer order":
		return "Layer vùng gom đang thiếu lệnh live nên bot đặt mới."
	case s == "order no longer matches active asset/layer":
		return "Lệnh không còn khớp coin/layer ACTIVE_LIMIT hiện tại nên bot hủy."
	case s == "order no longer matches current desired layer":
		return "Lệnh không còn khớp giá/vùng mua mới nên bot hủy hoặc thay thế."
	case s == "plan no longer ACTIVE_LIMIT":
		return "Plan không còn ACTIVE_LIMIT nên bot hủy lệnh chờ."
	case s == "per-asset open order limit reached":
		return "Coin này đã đạt số lệnh mở tối đa."
	case s == "total open order limit reached":
		return "Tổng số lệnh mở đã đạt giới hạn an toàn."
	case s == "per-asset live notional cap reached":
		return "Coin này đã đạt giới hạn vốn live."
	case s == "total live notional cap reached":
		return "Tổng vốn live đã đạt giới hạn an toàn."
	case s == "operator halt active":
		return "Operator halt đang bật; bot bị khóa quản lý lệnh thật."
	case s == "order placer/canceler unavailable":
		return "Không tạo được OKX client để đặt/hủy lệnh."
	case s == "preflight not pass":
		return "Preflight OKX chưa đạt tick size/lot size/min notional."
	case s == "order placer unavailable":
		return "Không tạo được OKX client để gửi lệnh."
	case s == "no deterministic ACTIVE_LIMIT layer available":
		return "Agent 2 chưa tạo layer ACTIVE_LIMIT nên không có lệnh hợp lệ."
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
		return "có dấu hiệu tích lũy, nhưng vẫn cần các gate khác xác nhận."
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
		b.WriteString("Không có live position trong ledger.\n")
	} else {
		b.WriteString("Live positions:\n")
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
	return "đang tắt — live có thể chạy nếu các gate khác cũng đạt"
}

func empty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func trimTelegram(s string) string {
	return textsafe.TrimTelegram(s, 3500)
}
