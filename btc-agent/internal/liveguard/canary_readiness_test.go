package liveguard

import "testing"

func TestEvaluateCanaryReadinessReady(t *testing.T) {
	r := EvaluateCanaryReadiness(CanaryReadinessInput{QualificationPassed: true, QualificationStressPassed: 100, QualificationStressRequired: 100, Doctor: RuntimeDoctorResult{Status: DoctorOK}, Reconcile: ReconcileSafetyResult{Status: ReconcileClean}, ExecutionAuthority: true})
	if r.Verdict != CanaryReady || len(r.Blockers) != 0 {
		t.Fatalf("expected ready: %+v", r)
	}
}
func TestEvaluateCanaryReadinessBlocksEveryControl(t *testing.T) {
	r := EvaluateCanaryReadiness(CanaryReadinessInput{QualificationStressRequired: 100, Doctor: RuntimeDoctorResult{Status: DoctorBlock}, Reconcile: ReconcileSafetyResult{Status: ReconcileBlock}, OperatorHalted: true, HermesDemoted: true, OpenLiveOrders: 1, HermesOwnedPositions: 1})
	if r.Verdict != CanaryBlocked || len(r.Blockers) != 9 {
		t.Fatalf("expected 9 blockers: %+v", r)
	}
}
