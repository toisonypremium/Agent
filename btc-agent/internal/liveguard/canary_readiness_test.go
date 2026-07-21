package liveguard

import (
	"testing"
	"time"
)

func TestEvaluateCanaryReadinessReady(t *testing.T) {
	r := EvaluateCanaryReadiness(CanaryReadinessInput{QualificationPassed: true, QualificationStressPassed: 100, QualificationStressRequired: 100, QualificationArtifact: "reports/qualification.json", QualificationSHA256: "abc123", QualificationAge: time.Hour, QualificationMaxAge: 30 * 24 * time.Hour, Doctor: RuntimeDoctorResult{Status: DoctorOK}, Reconcile: ReconcileSafetyResult{Status: ReconcileClean}, ExecutionAuthority: true})
	if r.Verdict != CanaryReady || len(r.Blockers) != 0 {
		t.Fatalf("expected ready: %+v", r)
	}
}

func TestEvaluateCanaryReadinessAllowsReconciledHermesInventory(t *testing.T) {
	r := EvaluateCanaryReadiness(CanaryReadinessInput{QualificationPassed: true, QualificationStressPassed: 100, QualificationStressRequired: 100, QualificationArtifact: "reports/qualification.json", QualificationSHA256: "abc123", QualificationAge: time.Hour, QualificationMaxAge: 30 * 24 * time.Hour, Doctor: RuntimeDoctorResult{Status: DoctorOK}, Reconcile: ReconcileSafetyResult{Status: ReconcileClean}, ExecutionAuthority: true, HermesOwnedPositions: 1})
	if r.Verdict != CanaryReady || len(r.Blockers) != 0 || r.HermesOwnedPositions != 1 {
		t.Fatalf("reconciled existing inventory must remain visible without blocking technical readiness: %+v", r)
	}
}

func TestEvaluateCanaryReadinessBlocksEveryControl(t *testing.T) {
	r := EvaluateCanaryReadiness(CanaryReadinessInput{QualificationStressRequired: 100, Doctor: RuntimeDoctorResult{Status: DoctorBlock}, Reconcile: ReconcileSafetyResult{Status: ReconcileBlock}, OperatorHalted: true, HermesDemoted: true, OpenLiveOrders: 1, HermesOwnedPositions: 1})
	if r.Verdict != CanaryBlocked || len(r.Blockers) < 8 {
		t.Fatalf("expected all authority and unresolved-order controls blocked: %+v", r)
	}
}

func TestValidateCanaryReadinessReportRequiresFreshCompleteEvidence(t *testing.T) {
	now := time.Now().UTC()
	report := CanaryReadinessResult{GeneratedAt: now, Verdict: CanaryReady, QualificationPassed: true, StressPassed: 100, StressRequired: 100, QualificationArtifact: "reports/qualification.json", QualificationSHA256: "abc123", DoctorStatus: DoctorOK, ReconcileStatus: ReconcileClean, ExecutionAuthority: true, HermesOwnedPositions: 1}
	if blockers := ValidateCanaryReadinessReport(report, now); len(blockers) != 0 {
		t.Fatalf("fresh report with reconciled inventory blocked: %v", blockers)
	}
	report.GeneratedAt = now.Add(-CanaryReadinessMaxAge - time.Second)
	if blockers := ValidateCanaryReadinessReport(report, now); len(blockers) == 0 {
		t.Fatal("stale report must block")
	}
}
