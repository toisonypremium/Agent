// Package dca contains pure policy only. It cannot read balances, mutate
// ledgers, submit orders, or clear an operator halt.
package dca

import "math"

type GateInput struct {
	MarketAllowed, BTCRiskHigh, LiquidityPass, ThesisFunded, RuntimeHealthy, ReconciliationClean, ArtifactFresh, OperatorHalted bool
	ThesisRemainingUSDT, ThesisMaxExposureUSDT, EnvelopeUSDT, GlobalCapPercent, GlobalUsedUSDT                                  float64
}
type GateResult struct {
	Allowed         bool
	Reason          string
	MaxNotionalUSDT float64
}

func EvaluateBuyGate(in GateInput) GateResult {
	for _, gate := range []struct {
		ok     bool
		reason string
	}{{!in.MarketAllowed, "market_not_allowed"}, {in.BTCRiskHigh, "btc_risk_high"}, {!in.LiquidityPass, "liquidity_gate_failed"}, {!in.ThesisFunded, "thesis_unfunded"}, {!in.RuntimeHealthy, "runtime_unhealthy"}, {!in.ReconciliationClean, "reconciliation_not_clean"}, {!in.ArtifactFresh, "okx_artifact_not_fresh"}, {in.OperatorHalted, "operator_halted"}} {
		if gate.ok {
			return GateResult{Reason: gate.reason}
		}
	}
	layer := in.ThesisMaxExposureUSDT * .25
	globalRemaining := in.EnvelopeUSDT*in.GlobalCapPercent/100 - in.GlobalUsedUSDT
	max := math.Min(layer, math.Min(in.ThesisRemainingUSDT, math.Min(in.EnvelopeUSDT*.02, globalRemaining)))
	if max <= 0 || math.IsNaN(max) || math.IsInf(max, 0) {
		return GateResult{Reason: "exposure_cap_reached"}
	}
	return GateResult{Allowed: true, MaxNotionalUSDT: max}
}

type TerminalOutcome string

const (
	TerminalFilled    TerminalOutcome = "FILLED"
	TerminalExpired   TerminalOutcome = "EXPIRED"
	TerminalCancelled TerminalOutcome = "CANCELLED"
	TerminalRejected  TerminalOutcome = "REJECTED"
)

func NextGlobalCap(current float64, outcome TerminalOutcome, reconciled bool) float64 {
	if outcome != TerminalFilled || !reconciled {
		return current
	}
	return math.Min(100, current+20)
}
func CanOpenLayer(layer int, marketAllowed bool, reconciledLayers, filledLayers int) bool {
	return marketAllowed && layer >= 1 && layer <= 3 && reconciledLayers >= layer-1 && filledLayers >= layer-1
}

type AutoHaltInput struct {
	ConsecutiveErrors                                                                                int
	UnknownOutcome, ReconciliationMismatch, VerifiedUSDTDecreased, ThesisExceeded, GlobalCapExceeded bool
	ObserverStaleEpochs                                                                              int
}
type AutoHaltResult struct {
	Halt   bool
	Reason string
}

func EvaluateAutoHalt(in AutoHaltInput) AutoHaltResult {
	for _, r := range []struct {
		yes bool
		why string
	}{{in.ConsecutiveErrors >= 3, "three_consecutive_errors"}, {in.UnknownOutcome, "unknown_exchange_outcome"}, {in.ReconciliationMismatch, "reconciliation_mismatch"}, {in.VerifiedUSDTDecreased, "verified_usdt_decreased"}, {in.ObserverStaleEpochs >= 2, "observer_stale"}, {in.ThesisExceeded, "thesis_exposure_exceeded"}, {in.GlobalCapExceeded, "global_exposure_cap_exceeded"}} {
		if r.yes {
			return AutoHaltResult{true, r.why}
		}
	}
	return AutoHaltResult{}
}
