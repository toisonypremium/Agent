package liveguard

import (
	"strings"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
)

// HermesSafetyContext contains deterministic facts that Hermes cannot override.
// This adapter is intentionally proposal-only: it does not call the exchange.
type HermesSafetyContext struct {
	OperatorHalted             bool
	DataHealthy                bool
	ReconcileClean             bool
	OKXReady                   bool
	PanicSelling               bool
	PortfolioNotionalRemaining float64
	AssetNotionalRemaining     map[string]float64
	Autonomous                 bool
	TotalCapital               float64
	AccumulationPhase          string
	MarketRegime               string
	TrendScore                 float64
	MMConfidence               float64
	DataQuality                float64
	LiquidityQuality           map[string]float64
	PerOrderCap                float64
}

type HermesActionDecision struct {
	Action       hermesoperator.Action `json:"action"`
	Allowed      bool                  `json:"allowed"`
	NotionalUSDT float64               `json:"notional_usdt,omitempty"`
	Reasons      []string              `json:"reasons,omitempty"`
}

// EvaluateHermesActions applies the non-overridable safety envelope to already
// schema-validated Hermes actions. It is suitable for observe/shadow mode.
func EvaluateHermesActions(actions []hermesoperator.Action, safety HermesSafetyContext) []HermesActionDecision {
	out := make([]HermesActionDecision, 0, len(actions))
	for _, action := range actions {
		result := HermesActionDecision{Action: action}
		symbol := strings.ToUpper(strings.TrimSpace(action.Symbol))
		if action.Intent.IncreasesExposure() {
			if safety.PanicSelling && !safety.Autonomous {
				result.Reasons = append(result.Reasons, "panic selling hard block")
			}
			if safety.OperatorHalted {
				result.Reasons = append(result.Reasons, "operator halt active")
			}
			if !safety.DataHealthy {
				result.Reasons = append(result.Reasons, "data health not safe")
			}
			if !safety.ReconcileClean {
				result.Reasons = append(result.Reasons, "reconcile not clean")
			}
			if !safety.OKXReady {
				result.Reasons = append(result.Reasons, "OKX not ready")
			}
			if safety.PortfolioNotionalRemaining <= 0 {
				result.Reasons = append(result.Reasons, "portfolio notional exhausted")
			}
			remaining := safety.AssetNotionalRemaining[symbol]
			if remaining <= 0 {
				result.Reasons = append(result.Reasons, "asset notional exhausted")
			}
			if len(result.Reasons) == 0 {
				result.NotionalUSDT = action.RequestedNotionalUSDT
				if safety.Autonomous {
					liq := 1.0
					if safety.LiquidityQuality != nil && safety.LiquidityQuality[symbol] > 0 {
						liq = safety.LiquidityQuality[symbol]
					}
					sizing := CalculateAutonomousSizing(AutonomousSizingContext{
						TotalCapital: safety.TotalCapital, Confidence: action.Confidence, Intent: action.Intent,
						AccumulationPhase: safety.AccumulationPhase, MarketRegime: safety.MarketRegime,
						TrendScore: safety.TrendScore, MMConfidence: safety.MMConfidence,
						DataQuality: safety.DataQuality, LiquidityQuality: liq,
						RequestedNotional: action.RequestedNotionalUSDT, PerOrderCap: safety.PerOrderCap,
						AssetRemaining: remaining, PortfolioRemaining: safety.PortfolioNotionalRemaining,
					})
					result.NotionalUSDT = sizing.NotionalUSDT
				}
				if result.NotionalUSDT > safety.PortfolioNotionalRemaining {
					result.NotionalUSDT = safety.PortfolioNotionalRemaining
				}
				if result.NotionalUSDT > remaining {
					result.NotionalUSDT = remaining
				}
				if result.NotionalUSDT <= 0 {
					result.Reasons = append(result.Reasons, "allocated notional is zero")
				}
			}
		}
		if action.Intent == hermesoperator.IntentReduce || action.Intent == hermesoperator.IntentExitLimit {
			if !safety.ReconcileClean {
				result.Reasons = append(result.Reasons, "reconcile not clean")
			}
			if !safety.OKXReady {
				result.Reasons = append(result.Reasons, "OKX not ready")
			}
		}
		if len(result.Reasons) == 0 {
			result.Allowed = true
		}
		out = append(out, result)
	}
	return out
}

// BuildHermesShadowDesiredOrders converts allowed exposure-increasing actions
// into desired orders for audit and dry-run. Autonomous production execution
// uses BuildHermesDesiredOrders with production provenance and final assertions.
func BuildHermesShadowDesiredOrders(cfg config.Config, plan agent2.Plan, decisions []HermesActionDecision, filters []live.InstrumentFilter) ([]ManagedDesiredOrder, []ManagedOrderDecision) {
	return BuildHermesDesiredOrders(cfg, plan, "", false, decisions, filters)
}

// BuildHermesDesiredOrders converts safety-approved actions into exchange-ready
// desired orders. production=true only changes provenance; it does not submit.
func BuildHermesDesiredOrders(cfg config.Config, plan agent2.Plan, decisionID string, production bool, decisions []HermesActionDecision, filters []live.InstrumentFilter) ([]ManagedDesiredOrder, []ManagedOrderDecision) {
	assets := map[string]agent2.AssetPlan{}
	for _, asset := range plan.Assets {
		assets[strings.ToUpper(asset.Symbol)] = asset
	}
	desired := []ManagedDesiredOrder{}
	blocked := []ManagedOrderDecision{}
	for _, decision := range decisions {
		action := decision.Action
		symbol := strings.ToUpper(action.Symbol)
		if !decision.Allowed || !action.Intent.IncreasesExposure() {
			continue
		}
		asset, ok := assets[symbol]
		if !ok {
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes shadow: asset not present in deterministic plan"})
			continue
		}
		autonomous := production && cfg.HermesOperator.NormalizedMode() == "autonomous"
		if asset.State == agent2.StateNoTrade && !autonomous {
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes shadow: asset NO_TRADE"})
			continue
		}
		if autonomous && asset.State == agent2.StateNoTrade {
			if !asset.DiscountZone.Valid() || asset.RewardRisk < cfg.Risk.MinScoutRewardRisk {
				blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes autonomous: asset price/RR envelope invalid"})
				continue
			}
			if asset.LiquidityQuality.Enabled && !asset.LiquidityQuality.Pass {
				blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes autonomous: asset liquidity unsafe"})
				continue
			}
		}
		zoneHigh := asset.DiscountZone.High
		if cfg.Risk.DiscountZonePremiumPct > 0 {
			zoneHigh *= 1 + cfg.Risk.DiscountZonePremiumPct
		}
		if action.EntryPrice < asset.DiscountZone.Low || action.EntryPrice > zoneHigh {
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes shadow: entry outside deterministic price envelope"})
			continue
		}
		layer := agent2.Layer{Index: 1, Price: action.EntryPrice, Notional: decision.NotionalUSDT, Invalidation: action.Invalidation, Target: action.Target, RewardRisk: asset.RewardRisk, Reason: "Hermes " + string(action.Intent)}
		candidate, ok := candidateFromLayer(cfg, symbol, layer, decision.NotionalUSDT)
		if !ok {
			blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes shadow: invalid order candidate"})
			continue
		}
		instID := live.OKXInstID(symbol)
		if len(filters) > 0 {
			var preflight PreflightResult
			candidate, preflight = RunPreflight(cfg, candidate, filters)
			if !preflight.Pass {
				blocked = append(blocked, ManagedOrderDecision{Action: "block", Symbol: symbol, Reason: "Hermes shadow: preflight not pass"})
				continue
			}
			instID = firstNonEmptyString(preflight.InstID, instID)
		}
		source := "HERMES_SHADOW"
		if production {
			source = "HERMES_OPERATOR"
		}
		tier := OpportunityProbe
		if action.Intent == hermesoperator.IntentOpenLimit {
			tier = OpportunityNormal
		}
		if action.Intent == hermesoperator.IntentScaleLimit {
			tier = OpportunityStrong
		}
		desired = append(desired, ManagedDesiredOrder{Symbol: symbol, InstID: instID, LayerIndex: 1, Side: "BUY", Type: "limit", Price: candidate.Price, Quantity: candidate.Quantity, Notional: candidate.Notional, PostOnly: true, InvalidationPrice: asset.Invalidation, DiscountZone: asset.DiscountZone, Source: source, DecisionReason: strings.Join(action.ReasonCodes, ","), AllocationTier: string(tier), AllocationReason: "Hermes validated action", TargetPrice: action.Target, RewardRisk: asset.RewardRisk, LayerReason: "Hermes " + string(action.Intent), DecisionID: decisionID, Intent: string(action.Intent)})
	}
	return desired, blocked
}
