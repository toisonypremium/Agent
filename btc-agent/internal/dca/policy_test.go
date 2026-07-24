package dca

import "testing"

func TestEvaluateBuyGateFailsClosedAndCalculatesCanaryNotional(t *testing.T) {
	in := GateInput{MarketAllowed: true, BTCRiskHigh: false, LiquidityPass: true, ThesisFunded: true, RuntimeHealthy: true, ReconciliationClean: true, ArtifactFresh: true, OperatorHalted: false, ThesisRemainingUSDT: 640, ThesisMaxExposureUSDT: 640, EnvelopeUSDT: 1600, GlobalCapPercent: 20, GlobalUsedUSDT: 0}
	out := EvaluateBuyGate(in)
	if !out.Allowed || out.MaxNotionalUSDT != 32 {
		t.Fatalf("out=%+v", out)
	}
	in.OperatorHalted = true
	if out = EvaluateBuyGate(in); out.Allowed || out.Reason != "operator_halted" {
		t.Fatalf("out=%+v", out)
	}
}
func TestExposureCapRampsOnlyAfterReconciledFill(t *testing.T) {
	if got := NextGlobalCap(20, TerminalFilled, true); got != 40 {
		t.Fatal(got)
	}
	for _, outcome := range []TerminalOutcome{TerminalExpired, TerminalCancelled, TerminalRejected} {
		if got := NextGlobalCap(20, outcome, true); got != 20 {
			t.Fatalf("%s %.2f", outcome, got)
		}
	}
	if got := NextGlobalCap(80, TerminalFilled, true); got != 100 {
		t.Fatal(got)
	}
}
func TestAutoHaltReasons(t *testing.T) {
	for _, in := range []AutoHaltInput{{ConsecutiveErrors: 3}, {UnknownOutcome: true}, {ReconciliationMismatch: true}, {VerifiedUSDTDecreased: true}, {ObserverStaleEpochs: 2}, {ThesisExceeded: true}, {GlobalCapExceeded: true}} {
		if got := EvaluateAutoHalt(in); !got.Halt {
			t.Fatalf("in=%+v got=%+v", in, got)
		}
	}
	if got := EvaluateAutoHalt(AutoHaltInput{ConsecutiveErrors: 2}); got.Halt {
		t.Fatalf("got=%+v", got)
	}
}
func TestLayerRequiresPriorReconciledFill(t *testing.T) {
	if !CanOpenLayer(1, true, 0, 0) || CanOpenLayer(2, true, 0, 0) || !CanOpenLayer(2, true, 1, 1) || CanOpenLayer(3, true, 1, 1) || !CanOpenLayer(3, true, 2, 2) {
		t.Fatal("bad layer transition")
	}
}
