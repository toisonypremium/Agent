package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
)

type accumulationReadinessReport struct {
	GeneratedAt       time.Time                   `json:"generated_at"`
	BTCPermission     agent1.Permission           `json:"btc_permission"`
	BTCRegime         string                      `json:"btc_regime"`
	BTCRisk           agent1.Risk                 `json:"btc_risk"`
	BTCTrendScore     float64                     `json:"btc_trend_score"`
	PlanState         agent2.State                `json:"plan_state"`
	TargetAllocations map[string]float64          `json:"target_allocations"`
	Coins             []accumulationReadinessCoin `json:"coins"`
	Summary           string                      `json:"summary"`
}

type accumulationReadinessCoin struct {
	Symbol            string                     `json:"symbol"`
	AllocationPct     float64                    `json:"allocation_pct"`
	Tier              string                     `json:"tier"`
	Actionable        bool                       `json:"actionable"`
	State             agent2.State               `json:"state"`
	ReadinessScore    float64                    `json:"readiness_score"`
	Price             float64                    `json:"price"`
	Support           market.Zone                `json:"support"`
	Resistance        market.Zone                `json:"resistance"`
	DiscountGapPct    float64                    `json:"discount_gap_pct"`
	ZoneWidthPct      float64                    `json:"zone_width_pct"`
	ZoneQuality       string                     `json:"zone_quality"`
	RewardRisk        float64                    `json:"reward_risk"`
	RelativeReturnPct float64                    `json:"relative_return_pct"`
	RotationRank      int                        `json:"rotation_rank"`
	RotationScore     float64                    `json:"rotation_score"`
	FlowBias          flow.Bias                  `json:"flow_bias"`
	FlowBullScore     float64                    `json:"flow_bull_score"`
	MMCase            agent2.MMCase              `json:"mm_case"`
	MMScore           float64                    `json:"mm_score"`
	LiquidityGrade    string                     `json:"liquidity_grade"`
	LiquidityPass     bool                       `json:"liquidity_pass"`
	HardFails         []string                   `json:"hard_fails"`
	SoftWaits         []string                   `json:"soft_waits"`
	Missing           []string                   `json:"missing"`
	NextTrigger       string                     `json:"next_trigger"`
	LayerPreview      []accumulationLayerPreview `json:"layer_preview"`
}

type accumulationLayerPreview struct {
	Index      int     `json:"index"`
	Price      float64 `json:"price"`
	Fraction   float64 `json:"fraction"`
	Notional   float64 `json:"notional"`
	Quantity   float64 `json:"quantity"`
	RewardRisk float64 `json:"reward_risk"`
}

func runAccumulationReadiness(ctx context.Context, cfg config.Config, db *storage.DB) error {
	_ = ctx
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return err
	}
	btc1d, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return fmt.Errorf("load BTC benchmark for accumulation readiness: %w", err)
	}
	benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d, "BTCUSDT": btc1d}
	plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assets, benchmarks)
	report := buildAccumulationReadinessReport(cfg, analysis, plan)
	if err := saveJSONFile("reports", "accumulation_readiness_latest.json", report); err != nil {
		return err
	}
	md := accumulationReadinessMarkdown(report)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "accumulation_readiness_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func buildAccumulationReadinessReport(cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan) accumulationReadinessReport {
	report := accumulationReadinessReport{
		GeneratedAt:       time.Now(),
		BTCPermission:     analysis.ActionPermission,
		BTCRegime:         analysis.MarketRegime,
		BTCRisk:           analysis.RiskLevel,
		BTCTrendScore:     analysis.TrendScore,
		PlanState:         plan.State,
		TargetAllocations: map[string]float64{},
	}
	for _, sym := range cfg.Data.Symbols.Assets {
		report.TargetAllocations[sym] = cfg.Portfolio.Allocation[sym]
	}
	for _, c := range plan.Watchlist.Candidates {
		coin := accumulationReadinessCoin{
			Symbol:            c.Symbol,
			AllocationPct:     cfg.Portfolio.Allocation[c.Symbol] * 100,
			Tier:              c.Tier,
			Actionable:        c.Actionable,
			State:             c.State,
			ReadinessScore:    c.ReadinessScore,
			Price:             c.Price,
			Support:           c.Support,
			Resistance:        c.Resistance,
			DiscountGapPct:    c.DiscountGap,
			ZoneWidthPct:      c.ZoneWidthPct,
			ZoneQuality:       c.ZoneQuality,
			RewardRisk:        c.RewardRisk,
			RelativeReturnPct: c.RelativeReturn * 100,
			RotationRank:      c.RotationRank,
			RotationScore:     c.RotationScore,
			FlowBias:          c.FlowBias,
			FlowBullScore:     c.FlowBullScore,
			MMCase:            c.MMCase,
			MMScore:           c.MMScore,
			LiquidityGrade:    c.LiquidityQuality.Grade,
			LiquidityPass:     c.LiquidityQuality.Pass,
			Missing:           append([]string(nil), c.Missing...),
			NextTrigger:       c.NextTrigger,
			LayerPreview:      readinessLayerPreview(cfg, c),
		}
		coin.HardFails, coin.SoftWaits = readinessChecklistGroups(c.EntryChecklist)
		report.Coins = append(report.Coins, coin)
	}
	report.Summary = accumulationReadinessSummary(report)
	return report
}

func readinessChecklistGroups(items []agent2.EntryChecklistItem) ([]string, []string) {
	hard := []string{}
	soft := []string{}
	for _, item := range items {
		if item.Pass {
			continue
		}
		if item.Severity == agent2.EntryCheckHard {
			hard = append(hard, item.Name)
		} else {
			soft = append(soft, item.Name)
		}
	}
	return hard, soft
}

func readinessLayerPreview(cfg config.Config, c agent2.WatchCandidate) []accumulationLayerPreview {
	if !c.Support.Valid() || !c.Resistance.Valid() || c.Support.Low <= 0 || c.Resistance.High <= 0 || c.Price <= 0 {
		return nil
	}
	budget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[c.Symbol] * cfg.Risk.MaxTotalDeploymentPerCycle
	if maxBudget := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment; maxBudget > 0 && budget > maxBudget {
		budget = maxBudget
	}
	if budget <= 0 {
		return nil
	}
	prices := []float64{c.Support.High, c.Support.Mid(), c.Support.Low}
	invalidation := c.Support.Low * 0.985
	out := []accumulationLayerPreview{}
	for i, fraction := range cfg.Execution.LayerDistribution {
		px := prices[minIntMain(i, len(prices)-1)]
		risk := px - invalidation
		reward := c.Resistance.High - px
		if px <= 0 || risk <= 0 || reward <= 0 || fraction <= 0 {
			continue
		}
		notional := budget * fraction
		out = append(out, accumulationLayerPreview{Index: i + 1, Price: px, Fraction: fraction, Notional: notional, Quantity: notional / px, RewardRisk: reward / risk})
	}
	return out
}

func accumulationReadinessSummary(r accumulationReadinessReport) string {
	if len(r.Coins) == 0 {
		return "No configured accumulation targets found."
	}
	actionable := 0
	for _, c := range r.Coins {
		if c.Actionable {
			actionable++
		}
	}
	best := r.Coins[0]
	if actionable == 0 {
		return fmt.Sprintf("No coin ready; BTC gate=%s; closest=%s readiness=%.2f tier=%s.", r.BTCPermission, best.Symbol, best.ReadinessScore, best.Tier)
	}
	return fmt.Sprintf("Actionable coins=%d; BTC gate=%s; best=%s readiness=%.2f.", actionable, r.BTCPermission, best.Symbol, best.ReadinessScore)
}

func accumulationReadinessMarkdown(r accumulationReadinessReport) string {
	md := "ACCUMULATION READINESS\n\n"
	md += fmt.Sprintf("Generated: %s\n", r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"))
	md += fmt.Sprintf("BTC gate: %s | regime=%s risk=%s trend=%.1f | plan_state=%s\n", r.BTCPermission, r.BTCRegime, r.BTCRisk, r.BTCTrendScore, r.PlanState)
	md += "BTC là market gate/benchmark, không phải target gom.\n"
	md += "Targets: " + readinessTargetsLine(r) + "\n"
	md += "Summary: " + r.Summary + "\n\n"
	for i, c := range r.Coins {
		md += fmt.Sprintf("%d. %s — %s readiness=%.2f actionable=%v state=%s\n", i+1, c.Symbol, c.Tier, c.ReadinessScore, c.Actionable, c.State)
		md += fmt.Sprintf("- Allocation: %.1f%%\n", c.AllocationPct)
		md += fmt.Sprintf("- Price: %.8f | support: %.8f-%.8f | resistance: %.8f-%.8f | distance: %+.2f%%\n", c.Price, c.Support.Low, c.Support.High, c.Resistance.Low, c.Resistance.High, c.DiscountGapPct*100)
		md += fmt.Sprintf("- Zone: %s width %.2f%% | RR current: %.2f\n", c.ZoneQuality, c.ZoneWidthPct*100, c.RewardRisk)
		md += fmt.Sprintf("- Rotation: rank #%d score %.2f | relative vs BTC: %+.2f%%\n", c.RotationRank, c.RotationScore, c.RelativeReturnPct)
		md += fmt.Sprintf("- Flow/MM: %s bull=%.2f | %s score=%.1f\n", c.FlowBias, c.FlowBullScore, c.MMCase, c.MMScore)
		md += fmt.Sprintf("- Liquidity: %s pass=%v\n", emptyDefault(c.LiquidityGrade, "n/a"), c.LiquidityPass)
		md += "- Hard fails: " + readinessList(c.HardFails) + "\n"
		md += "- Soft waits: " + readinessList(c.SoftWaits) + "\n"
		if len(c.Missing) > 0 {
			md += "- Missing: " + strings.Join(c.Missing, "; ") + "\n"
		}
		md += "- Next: " + c.NextTrigger + "\n"
		if len(c.LayerPreview) > 0 {
			md += "- Preview layers if BTC gate opens (not orders):\n"
			for _, layer := range c.LayerPreview {
				md += fmt.Sprintf("  - L%d price %.8f notional %.2f qty %.8f RR %.2f\n", layer.Index, layer.Price, layer.Notional, layer.Quantity, layer.RewardRisk)
			}
		} else {
			md += "- Preview layers if BTC gate opens: n/a\n"
		}
		md += "\n"
	}
	md += "No order was placed. Report-only readiness snapshot.\n"
	return md
}

func readinessTargetsLine(r accumulationReadinessReport) string {
	parts := []string{}
	for _, c := range r.Coins {
		parts = append(parts, fmt.Sprintf("%s %.0f%%", c.Symbol, r.TargetAllocations[c.Symbol]*100))
	}
	return strings.Join(parts, ", ")
}

func readinessList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func minIntMain(a, b int) int {
	if a < b {
		return a
	}
	return b
}
