package liveguard

import (
	"fmt"
	"math"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type OpportunityTier string

const (
	OpportunityBlock  OpportunityTier = "BLOCK"
	OpportunityWatch  OpportunityTier = "WATCH"
	OpportunityProbe  OpportunityTier = "PROBE"
	OpportunityNormal OpportunityTier = "NORMAL"
	OpportunityStrong OpportunityTier = "STRONG"
)

type AllocationDecision struct {
	Symbol            string          `json:"symbol"`
	Tier              OpportunityTier `json:"tier"`
	Score             float64         `json:"score"`
	BTCMultiplier     float64         `json:"btc_multiplier"`
	QualityMultiplier float64         `json:"quality_multiplier"`
	QualityGrade      string          `json:"quality_grade,omitempty"`
	MaxLayers         int             `json:"max_layers"`
	BudgetUSDT        float64         `json:"budget_usdt"`
	PerOrderUSDT      float64         `json:"per_order_usdt"`
	Reason            string          `json:"reason"`
}

func AllocateLiveCapital(cfg config.Config, plan agent2.Plan, quality map[string]historyQualityScore, positions []live.LivePosition, openOrderSets ...[]live.OrderStatus) map[string]AllocationDecision {
	out := map[string]AllocationDecision{}
	btcMult := btcRiskMultiplier(plan.ActionPermission)
	positionCost := map[string]float64{}
	for _, p := range positions {
		positionCost[strings.ToUpper(p.Symbol)] += p.CostBasis
	}
	openNotional := map[string]float64{}
	openTotal := 0.0
	if len(openOrderSets) > 0 {
		for _, order := range openOrderSets[0] {
			notional := liveOrderNotional(order)
			symbol := strings.ToUpper(order.Symbol)
			if symbol == "" {
				symbol = live.InternalSymbol(order.InstID)
			}
			openNotional[symbol] += notional
			openTotal += notional
		}
	}

	weights := map[string]float64{}
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		decision := allocationDecisionForAsset(cfg, plan, asset, quality[symbol], btcMult, positionCost[symbol], openNotional[symbol])
		out[symbol] = decision
		if decision.Tier != OpportunityBlock && decision.Tier != OpportunityWatch && decision.BudgetUSDT >= 0 {
			weights[symbol] = decision.Score * decision.BTCMultiplier * decision.QualityMultiplier
		}
	}

	totalWeight := 0.0
	for _, weight := range weights {
		if weight > 0 {
			totalWeight += weight
		}
	}
	if totalWeight <= 0 {
		return out
	}

	availableTotal := normalizedMaxLiveNotionalTotal(cfg) - openTotal
	if availableTotal < 0 {
		availableTotal = 0
	}
	perAssetCap := normalizedMaxLiveNotionalPerAsset(cfg)
	perOrderCap := normalizedMaxLiveNotionalPerOrder(cfg)
	for symbol, weight := range weights {
		decision := out[symbol]
		if weight <= 0 {
			decision.Tier = OpportunityBlock
			decision.Reason = appendAllocationReason(decision.Reason, "zero opportunity weight")
			out[symbol] = decision
			continue
		}
		budget := availableTotal * weight / totalWeight
		if budget > perAssetCap {
			budget = perAssetCap
		}
		remainingAssetBudget := portfolioRemainingBudget(cfg, symbol, positionCost[symbol], openNotional[symbol])
		if budget > remainingAssetBudget {
			budget = remainingAssetBudget
		}
		if budget > perAssetCap-positionCost[symbol]-openNotional[symbol] {
			budget = perAssetCap - positionCost[symbol] - openNotional[symbol]
		}
		if budget <= 0 {
			decision.Tier = OpportunityBlock
			decision.MaxLayers = 0
			decision.BudgetUSDT = 0
			decision.Reason = appendAllocationReason(decision.Reason, "asset live notional budget exhausted")
			out[symbol] = decision
			continue
		}
		if maxBudget := perOrderCap * float64(decision.MaxLayers); decision.MaxLayers > 0 && budget > maxBudget {
			budget = maxBudget
		}
		decision.BudgetUSDT = finiteFloat(budget)
		decision.PerOrderUSDT = finiteFloat(math.Min(perOrderCap, budget/float64(maxInt(1, decision.MaxLayers))))
		decision.Reason = appendAllocationReason(decision.Reason, fmt.Sprintf("dynamic budget %.2f USDT from opportunity weight %.2f", decision.BudgetUSDT, weight))
		out[symbol] = decision
	}
	return out
}

func allocationDecisionForAsset(cfg config.Config, plan agent2.Plan, asset agent2.AssetPlan, quality historyQualityScore, btcMult, positionCost, openNotional float64) AllocationDecision {
	symbol := strings.ToUpper(asset.Symbol)
	grade := strings.ToUpper(strings.TrimSpace(quality.Grade))
	qMult, qMaxLayers, qReason := qualityMultiplier(grade)
	score := opportunityScore(asset, quality)
	maxLayers := tierMaxLayers(cfg, score)
	if qMaxLayers > 0 && maxLayers > qMaxLayers {
		maxLayers = qMaxLayers
	}
	decision := AllocationDecision{Symbol: symbol, Tier: opportunityTier(score), Score: finiteFloat(score), BTCMultiplier: btcMult, QualityMultiplier: qMult, QualityGrade: grade, MaxLayers: maxLayers, Reason: qReason}
	if asset.State != agent2.StateActiveLimit && asset.State != agent2.StateArmed {
		decision.Tier = OpportunityWatch
		decision.MaxLayers = 0
		decision.Reason = appendAllocationReason(decision.Reason, "asset not ACTIVE_LIMIT/ARMED")
		return decision
	}
	if asset.State == agent2.StateArmed || (plan.State != agent2.StateActiveLimit && btcMult < 1) {
		decision.Tier = OpportunityProbe
		decision.MaxLayers = minInt(maxLayers, 1)
		decision.Reason = appendAllocationReason(decision.Reason, "BTC permission reduced to probe risk budget")
	}
	if btcMult <= 0 {
		decision.Tier = OpportunityBlock
		decision.MaxLayers = 0
		decision.Reason = appendAllocationReason(decision.Reason, "BTC permission has zero live risk budget")
		return decision
	}
	if qMult <= 0 {
		decision.Tier = OpportunityBlock
		decision.MaxLayers = 0
		return decision
	}
	if portfolioRemainingBudget(cfg, symbol, positionCost, openNotional) <= 0 || normalizedMaxLiveNotionalPerAsset(cfg)-positionCost-openNotional <= 0 {
		decision.Tier = OpportunityBlock
		decision.MaxLayers = 0
		decision.Reason = appendAllocationReason(decision.Reason, "asset live notional budget exhausted")
		return decision
	}
	if decision.MaxLayers <= 0 {
		decision.Tier = OpportunityWatch
		decision.Reason = appendAllocationReason(decision.Reason, "score below live entry threshold")
	}
	return decision
}

func portfolioRemainingBudget(cfg config.Config, symbol string, positionCost, openNotional float64) float64 {
	allocation := cfg.Portfolio.Allocation[strings.ToUpper(symbol)]
	if allocation <= 0 || cfg.Portfolio.TotalCapital <= 0 {
		return 0
	}
	target := cfg.Portfolio.TotalCapital * allocation * cfg.Risk.MaxTotalDeploymentPerCycle
	if cfg.Risk.MaxSingleAssetDeployment > 0 {
		maxSingle := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment
		if target > maxSingle {
			target = maxSingle
		}
	}
	if cap := normalizedMaxLiveNotionalPerAsset(cfg); cap > 0 && target > cap {
		target = cap
	}
	remaining := target - positionCost - openNotional
	if remaining < 0 {
		return 0
	}
	return remaining
}

func liveOrderNotional(order live.OrderStatus) float64 {
	if order.Notional > 0 {
		return order.Notional
	}
	if order.Price > 0 && order.Quantity > 0 {
		return order.Price * order.Quantity
	}
	return 0
}

func btcRiskMultiplier(permission agent1.Permission) float64 {
	switch permission {
	case agent1.Allowed:
		return 1
	case agent1.Armed:
		return 0.35
	case agent1.Watch:
		return 0
	default:
		return 0
	}
}

func qualityMultiplier(grade string) (float64, int, string) {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A", "B":
		return 1, 3, "quality A/B full size"
	case "C":
		return 0.5, 2, "quality C reduced size"
	case "D":
		return 0, 0, "quality D blocked"
	case "NO_SAMPLE", "":
		return 0.25, 1, "quality missing/NO_SAMPLE probe size"
	default:
		return 0.25, 1, "quality unknown probe size"
	}
}

func opportunityScore(asset agent2.AssetPlan, quality historyQualityScore) float64 {
	if asset.State != agent2.StateActiveLimit && asset.State != agent2.StateArmed {
		return 0
	}
	score := 50.0
	if asset.DiscountZone.Valid() {
		score += 10
	}
	if asset.RewardRisk > 0 {
		score += math.Min(20, asset.RewardRisk/3.0*20)
	}
	if quality.Score > 0 {
		score += math.Min(20, quality.Score/100.0*20)
	}
	if asset.AssetFlowScore > 0 {
		score += math.Min(10, asset.AssetFlowScore*10)
	}
	if asset.RotationScore > 0 {
		score += math.Min(10, asset.RotationScore/100.0*10)
	}
	if score > 100 {
		score = 100
	}
	return finiteFloat(score)
}

func opportunityTier(score float64) OpportunityTier {
	switch {
	case score >= 80:
		return OpportunityStrong
	case score >= 65:
		return OpportunityNormal
	case score >= 45:
		return OpportunityProbe
	default:
		return OpportunityWatch
	}
}

func tierMaxLayers(cfg config.Config, score float64) int {
	cap := normalizedMaxAutoLayersPerAsset(cfg)
	switch opportunityTier(score) {
	case OpportunityStrong:
		return minInt(cap, 3)
	case OpportunityNormal:
		return minInt(cap, 2)
	case OpportunityProbe:
		return minInt(cap, 1)
	default:
		return 0
	}
}

func appendAllocationReason(base, extra string) string {
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return base + "; " + extra
}

func finiteFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
