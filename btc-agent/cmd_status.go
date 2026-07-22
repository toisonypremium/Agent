package main

import (
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/opsplan"
	"btc-agent/internal/paper"
	"btc-agent/internal/storage"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func formatStatus(cfg config.Config, db *storage.DB) (string, error) {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return "no analysis yet; run fetch then analyze", nil
	}
	plan, _ := db.LatestPlan()
	orders, err := db.OpenPaperOrders()
	if err != nil {
		return "", err
	}
	halted, _ := db.IsHalted()
	haltStr := "INACTIVE"
	if halted {
		haltStr = "ACTIVE"
	}
	out := fmt.Sprintf(`BTC Agent Status
- Operator halt: %s
- BTC: %s | permission %s
- Trend score: %.1f
- Risk: %s | falling knife %s | FOMO %s
- Support: %.2f - %.2f
- Deep support: %.2f - %.2f
- Resistance: %.2f - %.2f
- Accumulation: %.2f - %.2f
- Invalidation: %.2f - %.2f

Flow
- Bias: %s | score %.2f
- Daily: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- 4H: sweep_low=%v reclaim=%v absorption=%v failed_breakout=%v distribution=%v
- Summary: %s

Agent 2
- State: %s
`, haltStr, analysis.MarketRegime, analysis.ActionPermission, analysis.TrendScore, analysis.RiskLevel, analysis.FallingKnifeRisk, analysis.FomoRisk, analysis.PrimarySupportZone.Low, analysis.PrimarySupportZone.High, analysis.DeepSupportZone.Low, analysis.DeepSupportZone.High, analysis.ResistanceZone.Low, analysis.ResistanceZone.High, analysis.AccumulationZone.Low, analysis.AccumulationZone.High, analysis.InvalidationZone.Low, analysis.InvalidationZone.High, analysis.Flow.Bias, analysis.Flow.Score, analysis.Flow.Daily.SweepLow, analysis.Flow.Daily.ReclaimSupport, analysis.Flow.Daily.Absorption, analysis.Flow.Daily.FailedBreakout, analysis.Flow.Daily.Distribution, analysis.Flow.FourHour.SweepLow, analysis.Flow.FourHour.ReclaimSupport, analysis.Flow.FourHour.Absorption, analysis.Flow.FourHour.FailedBreakout, analysis.Flow.FourHour.Distribution, analysis.Flow.Summary, plan.State)
	if len(plan.Rotation) > 0 {
		out += "- Asset ranking:\n"
		for _, r := range plan.Rotation {
			out += fmt.Sprintf("  - #%d %s score %.2f rel %.2f%% flow %s | %s\n", r.Rank, r.Symbol, r.Score, r.RelativeReturn*100, r.FlowBias, r.Reason)
		}
	}
	if len(plan.Watchlist.Candidates) > 0 {
		out += "- Watchlist gần đạt điều kiện:\n"
		limit := len(plan.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range plan.Watchlist.Candidates[:limit] {
			out += fmt.Sprintf("  - %s readiness %.2f tier=%s actionable=%v | checklist=%s | next=%s\n", c.Symbol, c.ReadinessScore, c.Tier, c.Actionable, agent2.ChecklistSummary(c.EntryChecklist), c.NextTrigger)
		}
	}
	if len(plan.Assets) == 0 {
		out += "- Assets: chưa có kế hoạch chi tiết hoặc Agent 1 chưa ALLOWED\n"
	} else {
		for _, asset := range plan.Assets {
			out += fmt.Sprintf("- %s: %s | rank %d score %.2f | asset flow %s %.2f | RR %.2f | %s\n", asset.Symbol, asset.State, asset.RotationRank, asset.RotationScore, asset.AssetFlowBias, asset.AssetFlowScore, asset.RewardRisk, asset.Reason)
			if unlock := assetUnlockPath(asset); unlock != "" {
				out += "  unlock: " + unlock + "\n"
			}
			out += "  " + agent2.StrategyIntelligenceLine(cfg, asset, analysis.ActionPermission) + "\n"
		}
	}
	out += fmt.Sprintf("- Open paper orders: %d", len(orders))
	out += fmt.Sprintf("\n\nHermes Operator\n- Enabled: %v | mode: %s | execution authority: %v\n- TTL: %ds | confidence: %.2f | actions/cycle: %d\n- Caps: probe %.2f | action %.2f | portfolio %.2f USDT", cfg.HermesOperator.Enabled, cfg.HermesOperator.NormalizedMode(), cfg.HermesOperator.CanExecute(), cfg.HermesOperator.DecisionTTLSeconds, cfg.HermesOperator.MinConfidence, cfg.HermesOperator.MaxActionsPerCycle, cfg.HermesOperator.MaxProbeNotionalUSDT, cfg.HermesOperator.MaxActionNotionalUSDT, cfg.HermesOperator.MaxPortfolioExposureUSDT)
	if shadow, ok := loadHermesShadowStatus(); ok {
		out += fmt.Sprintf("\n- Last shadow: %s | validated=%d | safety=%d | rejected=%d", shadow.GeneratedAt, shadow.Validated, shadow.Safety, shadow.Rejected)
	}
	if counts, err := db.PaperOrderStatusCounts(); err == nil && len(counts) > 0 {
		out += fmt.Sprintf("\n- Paper orders: OPEN=%d FILLED=%d EXPIRED=%d CANCELLED=%d INVALIDATED=%d", counts[paper.StatusOpen], counts[paper.StatusFilled], counts[paper.StatusExpired], counts[paper.StatusCancelled], counts[paper.StatusInvalidated])
	}
	operations := opsplan.Build(cfg, analysis, plan)
	out += fmt.Sprintf("\n\nOperations\n- Urgency: %s | scan configured/recommended: %d/%d minutes\n- Capital: total %.2f | reserve %.2f | cycle cap %.2f | committed %.2f | available %.2f | executable now %.2f USDT\n- State fingerprint: %s", operations.Market.Urgency, operations.Monitoring.ConfiguredScanMinutes, operations.Monitoring.RecommendedScanMinutes, operations.Capital.TotalCapitalUSDT, operations.Capital.ReserveCashUSDT, operations.Capital.CycleDeploymentCapUSDT, operations.Capital.AlreadyCommittedUSDT, operations.Capital.AvailableCycleCapacityUSDT, operations.Capital.ExecutableNowUSDT, operations.Fingerprint)
	for _, asset := range operations.Capital.Assets {
		out += fmt.Sprintf("\n  - %s %s/%s ready %.0f%% execute %.2f opportunity %.2f | trigger %s", asset.Symbol, asset.State, asset.Tier, asset.Readiness*100, asset.ExecutableBudgetUSDT, asset.OpportunityBudgetUSDT, asset.NextTrigger)
	}
	return out, nil
}

func assetUnlockPath(asset agent2.AssetPlan) string {
	missing := []string{}
	for _, gate := range asset.SetupGates {
		if gate.Pass {
			continue
		}
		switch gate.Name {
		case agent2.EntryCheckAssetFlowEntry, agent2.EntryCheckMMAccumulation:
			missing = append(missing, "flow reclaim/absorption")
		case agent2.EntryCheckDiscountZone:
			missing = append(missing, fmt.Sprintf("price closer to support (gap %.1f%%)", asset.DiscountGapPct*100))
		case agent2.EntryCheckRewardRisk:
			missing = append(missing, fmt.Sprintf("RR >= target (now %.2f)", asset.RewardRisk))
		case agent2.EntryCheckFallingKnife:
			missing = append(missing, "falling-knife clears")
		case agent2.EntryCheckRotationRank, agent2.EntryCheckRotationScore:
			missing = append(missing, "rotation improves")
		}
	}
	for _, reason := range asset.Reasons {
		if reason.Code == agent2.ReasonBTCPermission || reason.Code == agent2.ReasonBTCDowntrend {
			missing = append([]string{"BTC permission ALLOWED"}, missing...)
			break
		}
	}
	missing = uniqueStringsMain(missing)
	if len(missing) > 4 {
		missing = missing[:4]
	}
	return strings.Join(missing, "; ")
}

type hermesShadowStatus struct {
	GeneratedAt string
	Validated   int
	Safety      int
	Rejected    int
}

func loadHermesShadowStatus() (hermesShadowStatus, bool) {
	b, err := os.ReadFile(filepath.Join(hermesReportDir, "hermes_shadow_decision_latest.json"))
	if err != nil {
		return hermesShadowStatus{}, false
	}
	var raw struct {
		GeneratedAt string `json:"generated_at"`
		Validation  struct {
			Actions []json.RawMessage `json:"Actions"`
			Reasons []string          `json:"Reasons"`
		} `json:"validation"`
		Safety []struct {
			Allowed bool `json:"allowed"`
		} `json:"safety"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return hermesShadowStatus{}, false
	}
	status := hermesShadowStatus{GeneratedAt: raw.GeneratedAt, Validated: len(raw.Validation.Actions), Safety: len(raw.Safety), Rejected: len(raw.Validation.Reasons)}
	for _, d := range raw.Safety {
		if !d.Allowed {
			status.Rejected++
		}
	}
	return status, true
}
