package dca

import (
	"fmt"
	"math"
	"strings"

	"btc-agent/internal/agent2"
)

// ThesisBinding is explicit persisted strategy provenance. It is never built
// from an OKX holding or a ticker symbol alone.
type ThesisBinding struct {
	ThesisID, Symbol               string
	MaxExposureUSDT, RemainingUSDT float64
	ReconciledFilledLayers         int
}
type CanaryInput struct {
	Plan     agent2.Plan
	Bindings []ThesisBinding
	Gate     GateInput
}
type CanaryIntent struct {
	ThesisID                  string
	Symbol                    string
	Layer                     int
	Price, Notional, Quantity float64
	Reason                    string
}

// BuildCanaryIntent picks one explicit thesis-bound Layer 1 planner limit. It
// is pure planning; it cannot reserve capital or submit an exchange order.
func BuildCanaryIntent(in CanaryInput) (CanaryIntent, error) {
	if !in.Gate.MarketAllowed {
		return CanaryIntent{}, fmt.Errorf("market_not_allowed")
	}
	if in.Plan.State != agent2.StateActiveLimit {
		return CanaryIntent{}, fmt.Errorf("plan_not_active_limit")
	}
	bindings := map[string]ThesisBinding{}
	for _, b := range in.Bindings {
		if strings.TrimSpace(b.ThesisID) == "" || strings.TrimSpace(b.Symbol) == "" {
			return CanaryIntent{}, fmt.Errorf("invalid explicit thesis binding")
		}
		bindings[strings.ToUpper(b.Symbol)] = b
	}
	for _, ap := range in.Plan.Assets {
		b, ok := bindings[strings.ToUpper(ap.Symbol)]
		plannerThesisID := strings.TrimSpace(ap.ThesisID)
		if !ok || plannerThesisID == "" || plannerThesisID != b.ThesisID || !CanOpenLayer(1, true, b.ReconciledFilledLayers, b.ReconciledFilledLayers) {
			continue
		}
		for _, layer := range ap.Layers {
			if layer.Index != 1 || layer.Price <= 0 || layer.Quantity <= 0 {
				continue
			}
			gate := in.Gate
			gate.ThesisFunded = b.RemainingUSDT > 0
			gate.ThesisMaxExposureUSDT = b.MaxExposureUSDT
			gate.ThesisRemainingUSDT = b.RemainingUSDT
			verdict := EvaluateBuyGate(gate)
			if !verdict.Allowed {
				return CanaryIntent{}, fmt.Errorf("%s", verdict.Reason)
			}
			notional := math.Min(layer.Notional, verdict.MaxNotionalUSDT)
			if notional <= 0 {
				return CanaryIntent{}, fmt.Errorf("canary_notional_zero")
			}
			return CanaryIntent{ThesisID: b.ThesisID, Symbol: b.Symbol, Layer: 1, Price: layer.Price, Notional: notional, Quantity: notional / layer.Price, Reason: "dca_canary_layer_1"}, nil
		}
	}
	return CanaryIntent{}, fmt.Errorf("no_eligible_layer_1")
}
