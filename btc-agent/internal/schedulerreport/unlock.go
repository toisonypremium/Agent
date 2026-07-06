package schedulerreport

import (
	"fmt"
	"math"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/flow"
)

func BuildUnlockConditions(analysis agent1.MarketAnalysis, plan agent2.Plan) []string {
	out := []string{}
	if analysis.TrendScore < 45 {
		out = append(out, fmt.Sprintf("Trend score cần +%.1f điểm để lên ARMED (45.0).", 45-analysis.TrendScore))
	} else {
		out = append(out, "Trend score đã đủ ARMED; còn cần flow/zone/RR và safety gate sạch.")
	}
	allowedRegime := analysis.MarketRegime == "ACCUMULATION" || analysis.MarketRegime == "WEAK_UPTREND" || analysis.MarketRegime == "RANGE"
	if analysis.TrendScore < 60 || !allowedRegime {
		out = append(out, fmt.Sprintf("Để ALLOWED cần trend >=60 và regime ACCUMULATION/WEAK_UPTREND/RANGE; hiện trend %.1f, regime %s.", analysis.TrendScore, analysis.MarketRegime))
	}
	flowOK := (analysis.Flow.Bias == flow.BiasAccumulation || analysis.Flow.Bias == flow.BiasBearTrap) && analysis.Flow.Score >= 0.25
	if !flowOK {
		gap := math.Max(0, 0.25-analysis.Flow.Score)
		out = append(out, fmt.Sprintf("Flow cần ACCUMULATION/BEAR_TRAP hoặc reclaim/absorption rõ; hiện %s %.2f, thiếu %.2f điểm.", analysis.Flow.Bias, analysis.Flow.Score, gap))
	}
	if plan.State != agent2.StateActiveLimit {
		out = append(out, "Coin cần vào discount zone, asset flow đạt, rotation ổn và reward/risk >= 3.00 để tạo ACTIVE_LIMIT.")
	}
	out = append(out, "WATCH không tạo probe; ARMED mới cho probe nhỏ; ALLOWED mới cho ladder.")
	return limitUnlockConditions(out, 6)
}

func limitUnlockConditions(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
