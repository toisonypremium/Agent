package liveguard

import (
	"sort"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liquidity"
)

func BuildManagedDesiredOrders(cfg config.Config, plan agent2.Plan, filters []live.InstrumentFilter, positions []live.LivePosition, openOrders []live.OrderStatus) ([]ManagedDesiredOrder, []ManagedOrderDecision) {
	return BuildManagedDesiredOrdersWithContext(cfg, plan, filters, positions, openOrders, ManagedExecutionContext{})
}

func BuildManagedDesiredOrdersWithContext(cfg config.Config, plan agent2.Plan, filters []live.InstrumentFilter, positions []live.LivePosition, openOrders []live.OrderStatus, execCtx ManagedExecutionContext) ([]ManagedDesiredOrder, []ManagedOrderDecision) {
	desired := []ManagedDesiredOrder{}
	blocked := []ManagedOrderDecision{}
	if plan.State != agent2.StateActiveLimit || plan.ActionPermission != agent1.Allowed {
		return desired, blocked
	}
	qualityBySymbol := loadHistoryQualityScores("reports/live_manager_history_latest.json")
	allocationBySymbol := AllocateLiveCapital(cfg, plan, qualityBySymbol, positions, openOrders)
	totalDesired := 0.0
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		if asset.State != agent2.StateActiveLimit {
			continue
		}
		allocation := allocationBySymbol[symbol]
		if cfg.Live.LiquidityGateEnabled && asset.LiquidityQuality.Enabled && !asset.LiquidityQuality.Pass {
			reason := "liquidity gate blocked: " + liquidity.FirstReason(asset.LiquidityQuality.Reasons)
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: reason})
			continue
		}
		if allocation.Tier == OpportunityBlock || allocation.MaxLayers <= 0 || allocation.BudgetUSDT <= 0 {
			reason := "live allocation blocked: " + allocation.Reason
			if strings.EqualFold(allocation.QualityGrade, "D") {
				reason = "live quality filter blocked D-grade coin"
			}
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: reason})
			continue
		}
		assetRemaining := allocation.BudgetUSDT
		layers := asset.Layers
		if len(layers) > allocation.MaxLayers {
			layers = layers[:allocation.MaxLayers]
		}
		for _, layer := range layers {
			if assetRemaining <= 0 || totalDesired >= normalizedMaxLiveNotionalTotal(cfg) {
				break
			}
			notional := allocation.PerOrderUSDT
			if notional <= 0 {
				notional = normalizedMaxLiveNotionalPerOrder(cfg)
			}
			if cap := normalizedMaxLiveNotionalPerOrder(cfg); notional > cap {
				notional = cap
			}
			if notional > assetRemaining {
				notional = assetRemaining
			}
			if remainingTotal := normalizedMaxLiveNotionalTotal(cfg) - totalDesired; notional > remainingTotal {
				notional = remainingTotal
			}
			candidate, ok := candidateFromLayer(cfg, symbol, layer, notional)
			if !ok {
				blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, LayerIndex: layer.Index, Reason: "invalid layer candidate"})
				continue
			}
			preflightCandidate := candidate
			instID := live.OKXInstID(symbol)
			if len(filters) > 0 {
				var preflight PreflightResult
				preflightCandidate, preflight = RunPreflight(cfg, candidate, filters)
				if !preflight.Pass {
					blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, LayerIndex: layer.Index, Reason: "preflight not pass"})
					continue
				}
				instID = firstNonEmptyString(preflight.InstID, instID)
			}
			d := ManagedDesiredOrder{Symbol: symbol, InstID: instID, LayerIndex: layer.Index, Side: "BUY", Type: "limit", Price: preflightCandidate.Price, Quantity: preflightCandidate.Quantity, Notional: preflightCandidate.Notional, PostOnly: preflightCandidate.PostOnly, InvalidationPrice: asset.Invalidation, DiscountZone: asset.DiscountZone, Source: preflightCandidate.Source, DecisionReason: asset.Reason, QualityScore: qualityBySymbol[symbol].Score, QualityGrade: qualityBySymbol[symbol].Grade, AllocationScore: allocation.Score, AllocationTier: string(allocation.Tier), AllocationReason: allocation.Reason, TargetPrice: layer.Target, RewardRisk: layer.RewardRisk, ExpiresAt: layer.ExpiresAt, LayerReason: layer.Reason}
			desired = append(desired, d)
			assetRemaining -= d.Notional
			totalDesired += d.Notional
		}
	}
	sort.SliceStable(desired, func(i, j int) bool {
		if desired[i].AllocationScore != desired[j].AllocationScore {
			return desired[i].AllocationScore > desired[j].AllocationScore
		}
		if desired[i].QualityScore != desired[j].QualityScore {
			return desired[i].QualityScore > desired[j].QualityScore
		}
		if desired[i].Symbol == desired[j].Symbol {
			return desired[i].LayerIndex < desired[j].LayerIndex
		}
		return desired[i].Symbol < desired[j].Symbol
	})
	if cfg.Live.FirstOrderQuarantineEnabled && firstOrderQuarantineAppliesWithContext(openOrders, positions, execCtx) && len(desired) > 0 {
		first := desired[0]
		if cfg.Live.FirstOrderMaxNotionalUSDT > 0 && first.Notional > cfg.Live.FirstOrderMaxNotionalUSDT {
			first.Notional = cfg.Live.FirstOrderMaxNotionalUSDT
			if first.Price > 0 {
				first.Quantity = first.Notional / first.Price
			}
		}
		first.AllocationReason = strings.TrimSpace(first.AllocationReason + "; first-order quarantine: single smallest live layer only")
		for _, extra := range desired[1:] {
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: extra.Symbol, LayerIndex: extra.LayerIndex, Desired: extra, Reason: "first-order quarantine: only one live order allowed until first order is reviewed"})
		}
		desired = []ManagedDesiredOrder{first}
	}
	return desired, blocked
}

type historyQualityScore struct {
	Score float64
	Grade string
}
