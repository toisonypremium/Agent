package liveguard

import (
	"fmt"
	"strings"
	"time"
)

const (
	CanaryReady   = "READY"
	CanaryBlocked = "BLOCKED"
)

type CanaryReadinessInput struct {
	QualificationPassed         bool
	QualificationStressPassed   int
	QualificationStressRequired int
	Doctor                      RuntimeDoctorResult
	Reconcile                   ReconcileSafetyResult
	OperatorHalted              bool
	HermesDemoted               bool
	ExecutionAuthority          bool
	OpenLiveOrders              int
	HermesOwnedPositions        int
}

type CanaryReadinessResult struct {
	GeneratedAt          time.Time `json:"generated_at"`
	Verdict              string    `json:"verdict"`
	QualificationPassed  bool      `json:"qualification_passed"`
	StressPassed         int       `json:"stress_passed"`
	StressRequired       int       `json:"stress_required"`
	DoctorStatus         string    `json:"doctor_status"`
	ReconcileStatus      string    `json:"reconcile_status"`
	OperatorHalted       bool      `json:"operator_halted"`
	HermesDemoted        bool      `json:"hermes_demoted"`
	ExecutionAuthority   bool      `json:"execution_authority"`
	OpenLiveOrders       int       `json:"open_live_orders"`
	HermesOwnedPositions int       `json:"hermes_owned_positions"`
	Blockers             []string  `json:"blockers,omitempty"`
	Summary              string    `json:"summary"`
}

func EvaluateCanaryReadiness(in CanaryReadinessInput) CanaryReadinessResult {
	r := CanaryReadinessResult{GeneratedAt: time.Now().UTC(), Verdict: CanaryReady, QualificationPassed: in.QualificationPassed, StressPassed: in.QualificationStressPassed, StressRequired: in.QualificationStressRequired, DoctorStatus: in.Doctor.Status, ReconcileStatus: in.Reconcile.Status, OperatorHalted: in.OperatorHalted, HermesDemoted: in.HermesDemoted, ExecutionAuthority: in.ExecutionAuthority, OpenLiveOrders: in.OpenLiveOrders, HermesOwnedPositions: in.HermesOwnedPositions}
	if !in.QualificationPassed {
		r.Blockers = append(r.Blockers, "synthetic lifecycle qualification not passed")
	}
	if in.QualificationStressRequired <= 0 || in.QualificationStressPassed < in.QualificationStressRequired {
		r.Blockers = append(r.Blockers, "synthetic lifecycle stress threshold not met")
	}
	if in.Doctor.Status == DoctorBlock {
		r.Blockers = append(r.Blockers, "runtime doctor blocked")
	}
	if in.Reconcile.Status != ReconcileClean {
		r.Blockers = append(r.Blockers, "reconcile not clean")
	}
	if in.OperatorHalted {
		r.Blockers = append(r.Blockers, "operator halt active")
	}
	if in.HermesDemoted {
		r.Blockers = append(r.Blockers, "Hermes circuit breaker demoted")
	}
	if !in.ExecutionAuthority {
		r.Blockers = append(r.Blockers, "Hermes execution authority disabled")
	}
	if in.OpenLiveOrders != 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("%d open live orders present", in.OpenLiveOrders))
	}
	if in.HermesOwnedPositions != 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("%d Hermes-owned positions present", in.HermesOwnedPositions))
	}
	if len(r.Blockers) > 0 {
		r.Verdict = CanaryBlocked
	}
	r.Summary = fmt.Sprintf("%s: blockers=%d qualification=%v stress=%d/%d doctor=%s reconcile=%s", r.Verdict, len(r.Blockers), r.QualificationPassed, r.StressPassed, r.StressRequired, r.DoctorStatus, r.ReconcileStatus)
	r.Blockers = uniqueCanaryStrings(r.Blockers)
	return r
}
func uniqueCanaryStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
