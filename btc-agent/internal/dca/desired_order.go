package dca

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

// DesiredOrderInput is a narrow bridge from guarded runtime facts to the pure
// DCA policy. The caller owns exchange/reconciliation authority; this adapter
// has no exchange client and cannot place an order.
type DesiredOrderInput struct {
	Plan agent2.Plan
	Gate GateInput
	Now  time.Time
}

// BuildDesiredOrders produces only explicit, thesis-bound, post-only Layer 1
// candidates. Missing allocation/capital state blocks rather than falling back
// to the generic live opportunity sizing path.
func BuildDesiredOrders(db *storage.DB, in DesiredOrderInput) ([]liveguard.ManagedDesiredOrder, []liveguard.ManagedOrderDecision, error) {
	if db == nil {
		return nil, nil, fmt.Errorf("DCA ledger unavailable")
	}
	epoch, err := db.LatestDCAAllocationEpoch()
	if err != nil {
		return nil, nil, fmt.Errorf("DCA allocation epoch unavailable: %w", err)
	}
	ledgers, err := db.ThesisCapitalLedgers()
	if err != nil {
		return nil, nil, err
	}
	cap, err := db.DCAExecutionState()
	if err != nil {
		return nil, nil, err
	}
	used := 0.0
	bindings := make([]ThesisBinding, 0, len(ledgers))
	for _, l := range ledgers {
		used += l.ReservedUSDT + l.FilledUSDT
		bindings = append(bindings, ThesisBinding{ThesisID: l.ThesisID, Symbol: l.Symbol, MaxExposureUSDT: l.MaxExposureUSDT, RemainingUSDT: l.RemainingDCAUSDT})
	}
	gate := in.Gate
	gate.EnvelopeUSDT = epoch.EnvelopeUSDT
	gate.GlobalCapPercent = cap.GlobalCapPercent
	gate.GlobalUsedUSDT = used
	intent, err := BuildCanaryIntent(CanaryInput{Plan: in.Plan, Bindings: bindings, Gate: gate})
	if err != nil {
		return nil, []liveguard.ManagedOrderDecision{{Action: "block", Reason: err.Error()}}, nil
	}
	asset := findAsset(in.Plan, intent.Symbol)
	if asset == nil || (asset.LiquidityQuality.Enabled && !asset.LiquidityQuality.Pass) {
		return nil, []liveguard.ManagedOrderDecision{{Action: "block", Symbol: intent.Symbol, Reason: "liquidity_gate_failed"}}, nil
	}
	if intent.Layer != 1 || intent.Price <= 0 || intent.Quantity <= 0 || intent.Notional <= 0 {
		return nil, nil, fmt.Errorf("invalid DCA canary intent")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	d := liveguard.ManagedDesiredOrder{ThesisID: intent.ThesisID, Symbol: strings.ToUpper(intent.Symbol), InstID: live.OKXInstID(intent.Symbol), LayerIndex: 1, Side: "BUY", Type: "limit", Price: intent.Price, Quantity: intent.Quantity, Notional: intent.Notional, PostOnly: true, InvalidationPrice: asset.Invalidation, DiscountZone: asset.DiscountZone, Source: intent.Reason, DecisionReason: asset.Reason, TargetPrice: firstLayerTarget(*asset), ExpiresAt: now.Add(240 * time.Minute), LayerReason: "DCA thesis-bound canary"}
	return []liveguard.ManagedDesiredOrder{d}, nil, nil
}
func findAsset(p agent2.Plan, s string) *agent2.AssetPlan {
	for i := range p.Assets {
		if strings.EqualFold(p.Assets[i].Symbol, s) {
			return &p.Assets[i]
		}
	}
	return nil
}
func firstLayerTarget(a agent2.AssetPlan) float64 {
	for _, l := range a.Layers {
		if l.Index == 1 {
			return l.Target
		}
	}
	return 0
}
