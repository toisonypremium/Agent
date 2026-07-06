package agent1

import "fmt"

func DailyReport(a MarketAnalysis, planSummary string) string {
	return fmt.Sprintf(`BTC DAILY MARKET BRIEF

1. Kết luận nhanh
- BTC đang ở regime: %s
- Xu hướng 1W/1D/4H: %s / %s / %s
- Thị trường: risk=%s, trend_score=%.1f
- Hôm nay có được phép tìm lệnh không? %s

2. Phân tích đa khung
- 1W: %s
- 1D: %s
- 4H: %s

3. Vùng BTC làm market gate, không phải target gom
- Hỗ trợ gần: %.2f - %.2f
- Hỗ trợ sâu: %.2f - %.2f
- Kháng cự: %.2f - %.2f
- Vùng xác nhận đáy/accumulation gate: %.2f - %.2f
- Vùng macro/stress benchmark: %.2f - %.2f
- Vùng invalidation: %.2f - %.2f

4. Rủi ro
- Falling knife risk: %s
- FOMO risk: %s
- Volatility risk: %s
- Sentiment risk: %s (%d)

5. MM / Liquidity Flow
- Bias: %s | score %.2f
- Daily: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- 4H: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- Kết luận flow: %s

6. Kế hoạch Agent 2
%s

7. Kịch bản
- Kịch bản chính: %s
- Kịch bản bullish: %s
- Kịch bản bearish: %s
- Điều kiện vô hiệu: BTC phá vùng invalidation hoặc regime chuyển PANIC_SELLING.

8. Kết luận hành động
- %s
`, a.MarketRegime, a.WeeklyBias, a.DailyBias, a.FourHourBias, a.RiskLevel, a.TrendScore, a.ActionPermission, a.WeeklyBias, a.DailyBias, a.FourHourBias, a.PrimarySupportZone.Low, a.PrimarySupportZone.High, a.DeepSupportZone.Low, a.DeepSupportZone.High, a.ResistanceZone.Low, a.ResistanceZone.High, a.AccumulationZone.Low, a.AccumulationZone.High, a.MacroAccumulationZone.Low, a.MacroAccumulationZone.High, a.InvalidationZone.Low, a.InvalidationZone.High, a.FallingKnifeRisk, a.FomoRisk, a.RiskLevel, a.FearGreed.Classification, a.FearGreed.Value, a.Flow.Bias, a.Flow.Score, a.Flow.Daily.SweepLow, a.Flow.Daily.ReclaimSupport, a.Flow.Daily.Absorption, a.Flow.Daily.FailedBreakout, a.Flow.Daily.Distribution, a.Flow.FourHour.SweepLow, a.Flow.FourHour.ReclaimSupport, a.Flow.FourHour.Absorption, a.Flow.FourHour.FailedBreakout, a.Flow.FourHour.Distribution, a.Flow.Summary, planSummary, a.ScenarioMain, a.ScenarioBullish, a.ScenarioBearish, a.ActionPermission)
}
