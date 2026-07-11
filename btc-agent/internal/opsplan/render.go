package opsplan

import (
	"fmt"
	"strings"
)

func Markdown(r Report) string {
	var b strings.Builder
	b.WriteString("# KẾ HOẠCH VẬN HÀNH BTC AGENT\n\n")
	b.WriteString(fmt.Sprintf("- Thời điểm: %s\n", r.GeneratedAt.Format("2006-01-02 15:04:05 MST")))
	b.WriteString(fmt.Sprintf("- Fingerprint trạng thái: `%s`\n", r.Fingerprint))
	b.WriteString(fmt.Sprintf("- Tóm tắt: %s\n\n", r.Summary))

	b.WriteString("## 1. Trạng thái thị trường và quyền hành động\n\n")
	b.WriteString(fmt.Sprintf("- BTC: %.2f USDT | regime %s | trend %.1f | flow %s %.2f\n", r.Market.BTCPrice, r.Market.Regime, r.Market.TrendScore, r.Market.FlowBias, r.Market.FlowScore))
	b.WriteString(fmt.Sprintf("- Accumulation: %s | score %.1f | trigger: %s\n", fallback(r.Market.AccumulationPhase, "n/a"), r.Market.AccumulationScore, fallback(r.Market.AccumulationTrigger, "n/a")))
	b.WriteString(fmt.Sprintf("- Permission: %s | plan: %s | risk: %s | falling knife: %s | FOMO: %s\n", r.Market.Permission, r.Market.PlanState, r.Market.Risk, r.Market.FallingKnifeRisk, r.Market.FOMORisk))
	b.WriteString(fmt.Sprintf("- Mức ưu tiên theo dõi: %s\n", r.Market.Urgency))
	if len(r.Market.CriticalReasons) > 0 {
		b.WriteString("- Cảnh báo: " + strings.Join(r.Market.CriticalReasons, "; ") + "\n")
	}
	b.WriteString(fmt.Sprintf("- Vùng BTC: support %.2f-%.2f | invalidation %.2f | resistance từ %.2f\n", r.Market.PrimarySupportLow, r.Market.PrimarySupportHigh, r.Market.InvalidationLow, r.Market.ResistanceLow))
	if r.Microstructure.Enabled {
		b.WriteString(fmt.Sprintf("- Microstructure: %s | fresh %d/%d | BTC taker %.1f%% | CVD %.2f | spread %.2fbps | OB %s | funding %.4f | basis %.2f%%\n", r.Microstructure.Status, r.Microstructure.FreshSymbols, r.Microstructure.RequiredFresh, r.Microstructure.BTCTakerBuy*100, r.Microstructure.BTCCVD, r.Microstructure.BTCSpreadBps, fallback(r.Microstructure.BTCOrderBook, "n/a"), r.Microstructure.BTCFunding, r.Microstructure.BTCBasisPct))
		if len(r.Microstructure.Blockers) > 0 {
			b.WriteString("- Micro blockers: " + strings.Join(r.Microstructure.Blockers, "; ") + "\n")
		}
	}
	b.WriteString(fmt.Sprintf("- Kịch bản chính: %s\n", r.Market.MainScenario))
	b.WriteString(fmt.Sprintf("- Kịch bản mở khóa: %s\n", r.Market.UnlockScenario))
	b.WriteString(fmt.Sprintf("- Kịch bản vô hiệu: %s\n\n", r.Market.InvalidScenario))

	b.WriteString("## 2. Kế hoạch phân bổ vốn\n\n")
	b.WriteString(fmt.Sprintf("- Tổng vốn: %.2f USDT\n", r.Capital.TotalCapitalUSDT))
	b.WriteString(fmt.Sprintf("- Tiền mặt dự phòng: %.2f USDT\n", r.Capital.ReserveCashUSDT))
	b.WriteString(fmt.Sprintf("- Vốn có thể đầu tư: %.2f USDT\n", r.Capital.InvestableCapitalUSDT))
	b.WriteString(fmt.Sprintf("- Trần triển khai trong chu kỳ: %.2f USDT\n", r.Capital.CycleDeploymentCapUSDT))
	b.WriteString(fmt.Sprintf("- Đã cam kết: %.2f USDT (vị thế %.2f + lệnh mở %.2f) | còn khả dụng: %.2f USDT\n", r.Capital.AlreadyCommittedUSDT, r.Capital.ExistingPositionUSDT, r.Capital.OpenOrderNotionalUSDT, r.Capital.AvailableCycleCapacityUSDT))
	b.WriteString(fmt.Sprintf("- Có thể thực thi ngay: %.2f USDT | envelope cơ hội: %.2f USDT | chưa phân bổ: %.2f USDT\n", r.Capital.ExecutableNowUSDT, r.Capital.OpportunityReservedUSDT, r.Capital.UnusedCycleCapacityUSDT))
	b.WriteString(fmt.Sprintf("- Nguồn exposure: %s\n\n", fallback(r.Capital.ExposureSource, "không có")))
	b.WriteString("| Coin | State | Tier | Ready | Tỷ trọng | Trần | Đã dùng | Còn lại | Thực thi | Cơ hội | Layer | Trigger |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|---|\n")
	for _, a := range r.Capital.Assets {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %.0f%% | %.1f%% | %.2f | %.2f | %.2f | %.2f | %.2f | %s | %s |\n", a.Symbol, a.State, a.Tier, a.Readiness*100, a.TargetAllocationPct*100, a.StrategicCapUSDT, a.ExistingExposureUSDT, a.RemainingStrategicUSDT, a.ExecutableBudgetUSDT, a.OpportunityBudgetUSDT, formatLayers(a.LayerBudgetsUSDT), oneLine(a.NextTrigger)))
	}
	b.WriteString("\n" + r.Capital.Policy + "\n\n")

	b.WriteString("## 3. Kế hoạch theo dõi thị trường\n\n")
	b.WriteString(fmt.Sprintf("- Monitoring enabled: %v\n", r.Monitoring.Enabled))
	b.WriteString(fmt.Sprintf("- Chu kỳ quét cấu hình: %d phút | khuyến nghị hiện tại: %d phút\n", r.Monitoring.ConfiguredScanMinutes, r.Monitoring.RecommendedScanMinutes))
	b.WriteString(fmt.Sprintf("- Digest Telegram: %d phút | state-change: %v | critical: %v | nhắc critical: %d phút\n", r.Monitoring.TelegramDigestMinutes, r.Monitoring.NotifyOnStateChange, r.Monitoring.NotifyOnCritical, r.Monitoring.CriticalRepeatMinutes))
	b.WriteString("- Trọng tâm: " + strings.Join(r.Monitoring.Focus, "; ") + "\n\n")

	b.WriteString("## 4. Kế hoạch vận hành bot\n\n")
	b.WriteString(fmt.Sprintf("- Quyền thực thi: %s\n", r.Runtime.ExecutionAuthority))
	b.WriteString(fmt.Sprintf("- Daily run: %s | market scan: %d phút | reconcile: %d phút | supervisor: %d phút\n", r.Runtime.DailyRunTime, r.Monitoring.ConfiguredScanMinutes, r.Runtime.ReconcileIntervalMinutes, r.Runtime.ManagementIntervalMinutes))
	b.WriteString(fmt.Sprintf("- Live=%v | real=%v | auto=%v | supervisor=%v | order manager=%v | proof-only=%v\n", r.Runtime.LiveEnabled, r.Runtime.RealTradingEnabled, r.Runtime.AutoExecute, r.Runtime.SupervisorEnabled, r.Runtime.OrderManagementEnabled, r.Runtime.ProofOnly))
	b.WriteString("- An toàn: " + r.Runtime.SafetyPolicy + "\n\n")

	b.WriteString("## 5. Nội dung Telegram\n\n")
	b.WriteString("- Gửi ngay: " + strings.Join(r.Telegram.ImmediateEvents, "; ") + "\n")
	b.WriteString("- Digest: " + strings.Join(r.Telegram.DigestContents, "; ") + "\n")
	b.WriteString("- Chống nhiễu: " + r.Telegram.NoisePolicy + "\n")
	return b.String()
}

func TelegramDigest(r Report) string {
	var b strings.Builder
	b.WriteString("📊 BTC Agent — Kế hoạch vận hành\n\n")
	b.WriteString(fmt.Sprintf("I. THỊ TRƯỜNG\nBTC %.2f | %s | trend %.1f | flow %s %.2f\nAccumulation %s %.1f | Permission %s | Plan %s | Risk %s | ưu tiên %s\n", r.Market.BTCPrice, r.Market.Regime, r.Market.TrendScore, r.Market.FlowBias, r.Market.FlowScore, fallback(r.Market.AccumulationPhase, "n/a"), r.Market.AccumulationScore, r.Market.Permission, r.Market.PlanState, r.Market.Risk, r.Market.Urgency))
	if r.Microstructure.Enabled {
		b.WriteString(fmt.Sprintf("Micro %s fresh %d/%d | taker %.1f%% | CVD %.0f | OB %s | funding %.4f | basis %.2f%%\n", r.Microstructure.Status, r.Microstructure.FreshSymbols, r.Microstructure.RequiredFresh, r.Microstructure.BTCTakerBuy*100, r.Microstructure.BTCCVD, fallback(r.Microstructure.BTCOrderBook, "n/a"), r.Microstructure.BTCFunding, r.Microstructure.BTCBasisPct))
	}
	if len(r.Market.CriticalReasons) > 0 {
		b.WriteString("⚠️ " + strings.Join(r.Market.CriticalReasons, "; ") + "\n")
	}
	b.WriteString(fmt.Sprintf("Vùng: support %.2f-%.2f | invalid %.2f | resist %.2f\n\n", r.Market.PrimarySupportLow, r.Market.PrimarySupportHigh, r.Market.InvalidationLow, r.Market.ResistanceLow))

	b.WriteString("II. PHÂN BỔ VỐN\n")
	b.WriteString(fmt.Sprintf("Tổng %.2f | dự phòng %.2f | cap chu kỳ %.2f USDT\n", r.Capital.TotalCapitalUSDT, r.Capital.ReserveCashUSDT, r.Capital.CycleDeploymentCapUSDT))
	b.WriteString(fmt.Sprintf("Đã cam kết %.2f (vị thế %.2f + lệnh %.2f) | còn %.2f | thực thi %.2f USDT\n", r.Capital.AlreadyCommittedUSDT, r.Capital.ExistingPositionUSDT, r.Capital.OpenOrderNotionalUSDT, r.Capital.AvailableCycleCapacityUSDT, r.Capital.ExecutableNowUSDT))
	for _, a := range r.Capital.Assets {
		b.WriteString(fmt.Sprintf("- %s %s/%s | ready %.0f%% | đã dùng/còn %.2f/%.2f | thực thi %.2f | cơ hội %.2f | layer %s\n  trigger: %s\n", a.Symbol, a.State, a.Tier, a.Readiness*100, a.ExistingExposureUSDT, a.RemainingStrategicUSDT, a.ExecutableBudgetUSDT, a.OpportunityBudgetUSDT, formatLayers(a.LayerBudgetsUSDT), fallback(a.NextTrigger, "chưa có trigger cụ thể")))
	}

	b.WriteString("\nIII. THEO DÕI\n")
	b.WriteString(fmt.Sprintf("Quét %d phút (khuyến nghị %d) | digest %d phút | state-change=%v | critical=%v\n", r.Monitoring.ConfiguredScanMinutes, r.Monitoring.RecommendedScanMinutes, r.Monitoring.TelegramDigestMinutes, r.Monitoring.NotifyOnStateChange, r.Monitoring.NotifyOnCritical))
	b.WriteString("Theo dõi: BTC permission/flow, readiness+trigger, discount+RR, data/reconcile/order.\n")

	b.WriteString("\nIV. BOT & AN TOÀN\n")
	b.WriteString(r.Runtime.ExecutionAuthority + "\n")
	b.WriteString(fmt.Sprintf("Live=%v real=%v auto=%v supervisor=%v proof=%v\n", r.Runtime.LiveEnabled, r.Runtime.RealTradingEnabled, r.Runtime.AutoExecute, r.Runtime.SupervisorEnabled, r.Runtime.ProofOnly))
	b.WriteString("Spot-limit BUY post-only; không futures, không leverage, không market order.\n")
	b.WriteString("State: " + r.Fingerprint + "\n")
	return strings.TrimSpace(b.String())
}

func CriticalTelegram(r Report) string {
	var b strings.Builder
	b.WriteString("🚨 BTC Agent — Cảnh báo vận hành\n\n")
	b.WriteString(fmt.Sprintf("BTC %.2f | regime %s | accumulation %s | permission %s | plan %s\n", r.Market.BTCPrice, r.Market.Regime, fallback(r.Market.AccumulationPhase, "n/a"), r.Market.Permission, r.Market.PlanState))
	if len(r.Market.CriticalReasons) > 0 {
		b.WriteString("Lý do: " + strings.Join(r.Market.CriticalReasons, "; ") + "\n")
	}
	b.WriteString(fmt.Sprintf("Hành động: không mở rộng rủi ro; ngân sách thực thi %.2f USDT; đã cam kết %.2f; giữ dự phòng %.2f USDT.\n", r.Capital.ExecutableNowUSDT, r.Capital.AlreadyCommittedUSDT, r.Capital.ReserveCashUSDT))
	b.WriteString(fmt.Sprintf("Quét lại sau %d phút. Spot-limit only; không futures/leverage/market order.\n", r.Monitoring.RecommendedScanMinutes))
	b.WriteString("State: " + r.Fingerprint)
	return b.String()
}

func formatLayers(values []float64) string {
	if len(values) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%.2f", v))
	}
	return strings.Join(parts, "/")
}

func oneLine(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	v = strings.ReplaceAll(v, "|", "/")
	if len(v) > 120 {
		return v[:117] + "..."
	}
	return fallback(v, "—")
}

func fallback(v, fb string) string {
	if strings.TrimSpace(v) == "" {
		return fb
	}
	return v
}
