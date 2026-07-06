package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/backtest"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
)

type btcGateDiagnosticReport struct {
	GeneratedAt       time.Time                  `json:"generated_at"`
	Permission        agent1.Permission          `json:"permission"`
	PermissionReason  string                     `json:"permission_reason"`
	BTCPrice          float64                    `json:"btc_price"`
	Regime            string                     `json:"regime"`
	Risk              agent1.Risk                `json:"risk"`
	FallingKnifeRisk  agent1.Risk                `json:"falling_knife_risk"`
	FomoRisk          agent1.Risk                `json:"fomo_risk"`
	TrendScore        float64                    `json:"trend_score"`
	FlowBias          flow.Bias                  `json:"flow_bias"`
	FlowScore         float64                    `json:"flow_score"`
	FlowNextTrigger   string                     `json:"flow_next_trigger"`
	Support           market.Zone                `json:"support"`
	Resistance        market.Zone                `json:"resistance"`
	RewardRiskProxy   float64                    `json:"reward_risk_proxy"`
	FrameContribution []btcGateFrameContribution `json:"frame_contributions"`
	Policy            agent1.PermissionPolicy    `json:"policy"`
	UnlockConditions  []backtest.UnlockCondition `json:"unlock_conditions"`
	Summary           string                     `json:"summary"`
}

type btcGateFrameContribution struct {
	Timeframe        string  `json:"timeframe"`
	Bias             string  `json:"bias"`
	TrendScore       float64 `json:"trend_score"`
	Weight           float64 `json:"weight"`
	Contribution     float64 `json:"contribution"`
	EMA20            float64 `json:"ema20"`
	EMA50            float64 `json:"ema50"`
	EMA200           float64 `json:"ema200"`
	RSI14            float64 `json:"rsi14"`
	StructureLabel   string  `json:"structure_label"`
	LowerLowCount    int     `json:"lower_low_count"`
	HigherHighCount  int     `json:"higher_high_count"`
	BreakDown        bool    `json:"break_down"`
	BreakUp          bool    `json:"break_up"`
	LiquidityReclaim bool    `json:"liquidity_reclaim"`
}

func runBTCGateDiagnostic(ctx context.Context, cfg config.Config, db *storage.DB) error {
	_ = ctx
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	report := buildBTCGateDiagnosticReport(cfg, analysis)
	if err := saveJSONFile("reports", "btc_gate_diagnostic_latest.json", report); err != nil {
		return err
	}
	md := btcGateDiagnosticMarkdown(report)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "btc_gate_diagnostic_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func buildBTCGateDiagnosticReport(cfg config.Config, a agent1.MarketAnalysis) btcGateDiagnosticReport {
	policy := agent1.PermissionPolicyFromConfig(cfg)
	report := btcGateDiagnosticReport{
		GeneratedAt:       time.Now(),
		Permission:        a.ActionPermission,
		PermissionReason:  a.PermissionReason,
		BTCPrice:          a.BTCPrice,
		Regime:            a.MarketRegime,
		Risk:              a.RiskLevel,
		FallingKnifeRisk:  a.FallingKnifeRisk,
		FomoRisk:          a.FomoRisk,
		TrendScore:        a.TrendScore,
		FlowBias:          a.Flow.Bias,
		FlowScore:         a.Flow.Score,
		FlowNextTrigger:   firstNonEmpty(a.Flow.Daily.Diagnostics.NextBullTrigger, a.ScoreBreakdown.FlowNextTrigger),
		Support:           a.PrimarySupportZone,
		Resistance:        a.ResistanceZone,
		RewardRiskProxy:   btcGateRRProxy(a),
		FrameContribution: btcGateFrameContributions(a),
		Policy:            policy,
		UnlockConditions:  normalizeBTCGateUnlockConditions(btcGateUnlockConditions(a, policy)),
	}
	report.Summary = btcGateDiagnosticSummary(report)
	return report
}

func btcGateRRProxy(a agent1.MarketAnalysis) float64 {
	if !a.PrimarySupportZone.Valid() || !a.ResistanceZone.Valid() {
		return 0
	}
	entry := a.PrimarySupportZone.High
	invalidation := a.PrimarySupportZone.Low * 0.985
	risk := entry - invalidation
	if risk <= 0 {
		return 0
	}
	return (a.ResistanceZone.High - entry) / risk
}

func btcGateFrameContributions(a agent1.MarketAnalysis) []btcGateFrameContribution {
	defs := []struct {
		timeframe string
		weight    float64
	}{
		{timeframe: "1w", weight: 0.45},
		{timeframe: "1d", weight: 0.40},
		{timeframe: "4h", weight: 0.15},
	}
	out := []btcGateFrameContribution{}
	for _, def := range defs {
		frame := a.Frames[def.timeframe]
		out = append(out, btcGateFrameContribution{
			Timeframe:        def.timeframe,
			Bias:             frame.Bias,
			TrendScore:       frame.TrendScore,
			Weight:           def.weight,
			Contribution:     frame.TrendScore * def.weight,
			EMA20:            frame.EMA20,
			EMA50:            frame.EMA50,
			EMA200:           frame.EMA200,
			RSI14:            frame.RSI14,
			StructureLabel:   frame.Structure.Label,
			LowerLowCount:    frame.Structure.LowerLowCount,
			HigherHighCount:  frame.Structure.HigherHighCount,
			BreakDown:        frame.Structure.BreakDown,
			BreakUp:          frame.Structure.BreakUp,
			LiquidityReclaim: frame.Structure.LiquidityReclaim,
		})
	}
	return out
}

func btcGateDiagnosticSummary(r btcGateDiagnosticReport) string {
	armedGap := math.Max(0, r.Policy.TrendArmedThreshold-r.TrendScore)
	if r.Permission == agent1.Armed || r.Permission == agent1.Allowed {
		return fmt.Sprintf("BTC gate already %s; trend route unlocked, keep checking flow/risk before sizing.", r.Permission)
	}
	flowPromote := (r.FlowBias == flow.BiasAccumulation || r.FlowBias == flow.BiasBearTrap) && r.FlowScore >= r.Policy.FlowPromoteThreshold
	if armedGap == 0 && !flowPromote {
		return fmt.Sprintf("%s because hard or reward/risk blockers still apply; trend route is not the bottleneck.", r.Permission)
	}
	if flowPromote {
		return fmt.Sprintf("%s despite flow route being ready; inspect hard blockers before probe.", r.Permission)
	}
	return fmt.Sprintf("%s because trend is %.2f points below ARMED and flow has no accumulation/bear-trap confirmation. Do not accumulate until trend route or flow route unlocks.", r.Permission, armedGap)
}

func btcGateUnlockConditions(a agent1.MarketAnalysis, policy agent1.PermissionPolicy) []backtest.UnlockCondition {
	items := backtest.PermissionUnlockConditions(a)
	for i := range items {
		switch items[i].Name {
		case "TREND_TO_ARMED":
			items[i].Target = fmt.Sprintf("%.1f", policy.TrendArmedThreshold)
			items[i].Gap = math.Max(0, policy.TrendArmedThreshold-a.TrendScore)
			items[i].Pass = items[i].Gap == 0
			items[i].Reason = fmt.Sprintf("trend score cần +%.1f điểm để lên ARMED", items[i].Gap)
		case "TREND_TO_ALLOWED":
			items[i].Target = fmt.Sprintf("%.1f", policy.TrendAllowedThreshold)
			items[i].Gap = math.Max(0, policy.TrendAllowedThreshold-a.TrendScore)
			items[i].Pass = items[i].Gap == 0
			items[i].Reason = fmt.Sprintf("trend score cần +%.1f điểm để đủ ALLOWED", items[i].Gap)
		case "FLOW_PROMOTE_ARMED":
			items[i].Target = fmt.Sprintf("ACCUMULATION/BEAR_TRAP >=%.2f", policy.FlowPromoteThreshold)
			items[i].Gap = math.Max(0, policy.FlowPromoteThreshold-a.Flow.Score)
			items[i].Pass = policy.FlowPromotesToArmed(a.Flow)
		case "RR_PROXY":
			items[i].Target = fmt.Sprintf("%.2f", policy.PermissionMinRewardRisk)
			items[i].Gap = math.Max(0, policy.PermissionMinRewardRisk-btcGateRRProxy(a))
			items[i].Pass = items[i].Gap == 0
		}
	}
	return items
}

func normalizeBTCGateUnlockConditions(items []backtest.UnlockCondition) []backtest.UnlockCondition {
	out := make([]backtest.UnlockCondition, 0, len(items))
	for _, item := range items {
		if item.Name == "HARD_BLOCKERS" && item.Pass {
			item.Current = "none"
		}
		out = append(out, item)
	}
	return out
}

func btcGateDiagnosticMarkdown(r btcGateDiagnosticReport) string {
	armedGap := math.Max(0, r.Policy.TrendArmedThreshold-r.TrendScore)
	allowedGap := math.Max(0, r.Policy.TrendAllowedThreshold-r.TrendScore)
	md := "BTC GATE DIAGNOSTIC\n\n"
	md += fmt.Sprintf("Generated: %s\n", r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"))
	md += fmt.Sprintf("Current: %s | reason=%s\n", r.Permission, emptyDefault(r.PermissionReason, "n/a"))
	md += fmt.Sprintf("Regime/Risk: %s | risk=%s falling=%s fomo=%s\n", r.Regime, r.Risk, r.FallingKnifeRisk, r.FomoRisk)
	md += fmt.Sprintf("BTC price: %.8f\n", r.BTCPrice)
	md += fmt.Sprintf("Support: %.8f-%.8f | Resistance: %.8f-%.8f | RR proxy: %.2f\n\n", r.Support.Low, r.Support.High, r.Resistance.Low, r.Resistance.High, r.RewardRiskProxy)
	md += "Trend route:\n"
	md += fmt.Sprintf("- Trend score: %.2f\n", r.TrendScore)
	md += fmt.Sprintf("- Gap to ARMED: +%.2f (target %.2f)\n", armedGap, r.Policy.TrendArmedThreshold)
	md += fmt.Sprintf("- Gap to ALLOWED: +%.2f (target %.2f)\n\n", allowedGap, r.Policy.TrendAllowedThreshold)
	md += "Frame contribution:\n"
	for _, frame := range r.FrameContribution {
		md += fmt.Sprintf("- %s: trend=%.2f weight=%.0f%% contribution=%.2f bias=%s %s RSI=%.1f structure=%s\n", frame.Timeframe, frame.TrendScore, frame.Weight*100, frame.Contribution, emptyDefault(frame.Bias, "UNKNOWN"), frameEMANote(frame, r.BTCPrice), frame.RSI14, emptyDefault(frame.StructureLabel, "UNKNOWN"))
	}
	md += "\nFlow route:\n"
	md += fmt.Sprintf("- Flow: %s score=%.2f\n", r.FlowBias, r.FlowScore)
	md += fmt.Sprintf("- Promote to ARMED requires ACCUMULATION/BEAR_TRAP >=%.2f\n", r.Policy.FlowPromoteThreshold)
	md += "- Next trigger: " + emptyDefault(r.FlowNextTrigger, "n/a") + "\n\n"
	md += "Unlock checklist:\n"
	for _, item := range r.UnlockConditions {
		md += fmt.Sprintf("- %s: %s current=%s target=%s gap=%.2f — %s\n", item.Name, passFail(item.Pass), emptyDefault(item.Current, "n/a"), emptyDefault(item.Target, "n/a"), item.Gap, item.Reason)
	}
	md += "\nSummary: " + r.Summary + "\n\n"
	md += "No order was placed. Report-only BTC gate diagnostic.\n"
	return md
}

func frameEMANote(frame btcGateFrameContribution, price float64) string {
	parts := []string{}
	if frame.EMA20 > 0 && price > 0 {
		if price > frame.EMA20 {
			parts = append(parts, "close>EMA20")
		} else {
			parts = append(parts, "close<EMA20")
		}
	}
	if frame.EMA50 > 0 && frame.EMA20 > 0 {
		if frame.EMA20 > frame.EMA50 {
			parts = append(parts, "EMA20>EMA50")
		} else {
			parts = append(parts, "EMA20<EMA50")
		}
	}
	if frame.EMA200 > 0 && price > 0 {
		if price > frame.EMA200 {
			parts = append(parts, "close>EMA200")
		} else {
			parts = append(parts, "close<EMA200")
		}
	}
	if frame.BreakUp {
		parts = append(parts, "break_up")
	}
	if frame.BreakDown {
		parts = append(parts, "break_down")
	}
	if frame.LiquidityReclaim {
		parts = append(parts, "liquidity_reclaim")
	}
	if len(parts) == 0 {
		return "n/a"
	}
	return strings.Join(parts, "/")
}

func passFail(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}
