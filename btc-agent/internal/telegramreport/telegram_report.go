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
	b.WriteString("🤖 BTC Agent — Báo cáo ngày\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", ExplainPermission(analysis.ActionPermission)))
	b.WriteString("Lệnh thật: KHÔNG đặt lệnh từ báo cáo này.\n\n")
	b.WriteString("1) BTC đang thế nào?\n")
	b.WriteString(fmt.Sprintf("- Regime: %s.\n", analysis.MarketRegime))
	b.WriteString(fmt.Sprintf("- Risk: %s. Falling knife: %s. FOMO: %s.\n", ExplainRisk(analysis.RiskLevel), analysis.FallingKnifeRisk, analysis.FomoRisk))
	b.WriteString(fmt.Sprintf("- Trend score: %.1f. Flow: %s %.2f — %s\n", analysis.TrendScore, analysis.Flow.Bias, analysis.Flow.Score, humanFlow(analysis.Flow.Bias)))
	b.WriteString("\n2) Agent 2 đang làm gì?\n")
	b.WriteString(fmt.Sprintf("- Plan: %s — %s\n", plan.State, ExplainPlanState(plan.State)))
	if len(plan.Watchlist.Candidates) > 0 {
		b.WriteString("- Watchlist gần nhất:\n")
		for _, c := range firstCandidates(plan.Watchlist.Candidates, 3) {
			b.WriteString(fmt.Sprintf("  • %s readiness %.2f: %s\n", c.Symbol, c.ReadinessScore, ExplainPlanState(c.State)))
			if len(c.Missing) > 0 {
				b.WriteString(fmt.Sprintf("    Thiếu: %s\n", humanList(c.Missing, 3)))
			}
			if c.NextTrigger != "" {
				b.WriteString(fmt.Sprintf("    Chờ: %s\n", c.NextTrigger))
			}
		}
	} else {
		b.WriteString("- Chưa có watchlist đủ gần điều kiện.\n")
	}
	b.WriteString("\n3) Hành động an toàn\n")
	b.WriteString("- Tiếp tục theo dõi. Không resume live, không đặt lệnh nếu chưa có ACTIVE_LIMIT và proof sạch.\n")
	return trimTelegram(b.String())
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
	b.WriteString("🤖 BTC Agent — Research brief\n\n")
	b.WriteString(fmt.Sprintf("Kết luận: %s\n", result.Summary))
	b.WriteString("Research-only: không đặt lệnh, không override Agent 1/2.\n")
	limit := len(result.Items)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		item := result.Items[i]
		b.WriteString(fmt.Sprintf("\n%d) [%s] %s\n", i+1, item.Risk, item.Title))
		if len(item.Tags) > 0 {
			b.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(item.Tags, ", ")))
		}
		b.WriteString(fmt.Sprintf("Source: %s\n%s\n", item.Source, item.URL))
	}
	if len(result.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, warning := range result.Warnings {
			b.WriteString("- " + warning + "\n")
		}
	}
	return trimTelegram(b.String())
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
	const max = 3500
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s + "\n"
	}
	return strings.TrimSpace(s[:max]) + "\n...\n"
}
