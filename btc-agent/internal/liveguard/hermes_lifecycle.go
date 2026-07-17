package liveguard

import (
	"fmt"
	"strings"

	"btc-agent/internal/agent2"
	"btc-agent/internal/hermesoperator"
)

// HermesLifecycleContext is the deterministic evidence and portfolio state used
// to enforce PROBE -> OPEN -> SCALE ordering. It complements, not replaces,
// account/data/reconcile safety.
type HermesLifecycleContext struct {
	Action           hermesoperator.Action
	Asset            agent2.AssetPlan
	ExistingNotional float64
	AssetCap         float64
	HasOpenBuy       bool
}

type HermesLifecycleResult struct {
	Allowed       bool     `json:"allowed"`
	Stage         string   `json:"stage"`
	Confirmations int      `json:"confirmations"`
	Required      int      `json:"required"`
	Reasons       []string `json:"reasons,omitempty"`
}

func EvaluateHermesLifecycle(in HermesLifecycleContext) HermesLifecycleResult {
	r := HermesLifecycleResult{Stage: string(in.Action.Intent)}
	if !in.Action.Intent.IncreasesExposure() {
		r.Allowed = true
		return r
	}
	if in.HasOpenBuy {
		r.Reasons = append(r.Reasons, "pending buy order must resolve before next stage")
	}
	if !in.Asset.DiscountZone.Valid() || in.Action.EntryPrice < in.Asset.DiscountZone.Low || in.Action.EntryPrice > in.Asset.DiscountZone.High*1.01 {
		r.Reasons = append(r.Reasons, "price outside accumulation envelope")
	}
	if in.Action.Invalidation <= 0 || in.Action.Invalidation >= in.Action.EntryPrice {
		r.Reasons = append(r.Reasons, "valid downside invalidation required")
	}
	if in.Action.Target <= in.Action.EntryPrice {
		r.Reasons = append(r.Reasons, "valid upside target required")
	}
	if in.Asset.LiquidityQuality.Enabled && !in.Asset.LiquidityQuality.Pass {
		r.Reasons = append(r.Reasons, "liquidity not safe")
	}

	confirm := 0
	if in.Asset.MMScore >= 50 {
		confirm++
	}
	if in.Asset.AssetFlowScore >= 0.15 {
		confirm++
	}
	if in.Asset.SetupScore >= 0.60 {
		confirm++
	}
	if in.Asset.State == agent2.StateArmed || in.Asset.State == agent2.StateActiveLimit {
		confirm++
	}
	if in.Asset.RewardRisk >= 3 {
		confirm++
	}
	r.Confirmations = confirm
	exposureRatio := 0.0
	if in.AssetCap > 0 {
		exposureRatio = in.ExistingNotional / in.AssetCap
	}
	switch in.Action.Intent {
	case hermesoperator.IntentProbeLimit:
		r.Required = 1
		if in.ExistingNotional > 0 {
			r.Reasons = append(r.Reasons, "probe stage already completed")
		}
		if in.Asset.RewardRisk < 1.5 {
			r.Reasons = append(r.Reasons, "probe reward/risk below 1.5")
		}
	case hermesoperator.IntentOpenLimit:
		r.Required = 2
		if in.ExistingNotional <= 0 {
			r.Reasons = append(r.Reasons, "cannot open before a filled probe")
		}
		if exposureRatio >= 0.60 {
			r.Reasons = append(r.Reasons, "open stage already sufficiently allocated")
		}
	case hermesoperator.IntentScaleLimit:
		r.Required = 4
		if in.ExistingNotional <= 0 {
			r.Reasons = append(r.Reasons, "cannot scale without an owned position")
		}
		if exposureRatio < 0.15 {
			r.Reasons = append(r.Reasons, "position has not completed confirmation stage")
		}
		if in.Asset.State != agent2.StateArmed && in.Asset.State != agent2.StateActiveLimit {
			r.Reasons = append(r.Reasons, "scale requires armed or active setup")
		}
	}
	if confirm < r.Required {
		r.Reasons = append(r.Reasons, fmt.Sprintf("confirmation count %d below %d", confirm, r.Required))
	}
	r.Allowed = len(r.Reasons) == 0
	return r
}

func LifecycleReasonText(reasons []string) string { return strings.Join(reasons, "; ") }
