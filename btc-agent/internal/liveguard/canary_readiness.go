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
	QualificationArtifact       string
	QualificationSHA256         string
	QualificationAge            time.Duration
	QualificationMaxAge         time.Duration
	Doctor                      RuntimeDoctorResult
	Reconcile                   ReconcileSafetyResult
	OperatorHalted              bool
	HermesDemoted               bool
	ExecutionAuthority          bool
	OpenLiveOrders              int
	HermesOwnedPositions        int
}

type CanaryReadinessResult struct {
	GeneratedAt           time.Time `json:"generated_at"`
	Verdict               string    `json:"verdict"`
	QualificationPassed   bool      `json:"qualification_passed"`
	StressPassed          int       `json:"stress_passed"`
	StressRequired        int       `json:"stress_required"`
	QualificationArtifact string    `json:"qualification_artifact,omitempty"`
	QualificationSHA256   string    `json:"qualification_sha256,omitempty"`
	QualificationAgeHours float64   `json:"qualification_age_hours,omitempty"`
	DoctorStatus          string    `json:"doctor_status"`
	ReconcileStatus       string    `json:"reconcile_status"`
	OperatorHalted        bool      `json:"operator_halted"`
	HermesDemoted         bool      `json:"hermes_demoted"`
	ExecutionAuthority    bool      `json:"execution_authority"`
	OpenLiveOrders        int       `json:"open_live_orders"`
	HermesOwnedPositions  int       `json:"hermes_owned_positions"`
	Blockers              []string  `json:"blockers,omitempty"`
	Summary               string    `json:"summary"`
}

func EvaluateCanaryReadiness(in CanaryReadinessInput) CanaryReadinessResult {
	r := CanaryReadinessResult{GeneratedAt: time.Now().UTC(), Verdict: CanaryReady, QualificationPassed: in.QualificationPassed, StressPassed: in.QualificationStressPassed, StressRequired: in.QualificationStressRequired, QualificationArtifact: in.QualificationArtifact, QualificationSHA256: in.QualificationSHA256, QualificationAgeHours: in.QualificationAge.Hours(), DoctorStatus: in.Doctor.Status, ReconcileStatus: in.Reconcile.Status, OperatorHalted: in.OperatorHalted, HermesDemoted: in.HermesDemoted, ExecutionAuthority: in.ExecutionAuthority, OpenLiveOrders: in.OpenLiveOrders, HermesOwnedPositions: in.HermesOwnedPositions}
	if !in.QualificationPassed {
		r.Blockers = append(r.Blockers, "synthetic lifecycle qualification not passed")
	}
	if in.QualificationArtifact == "" || in.QualificationSHA256 == "" {
		r.Blockers = append(r.Blockers, "synthetic lifecycle qualification provenance unavailable")
	}
	if in.QualificationAge < 0 || (in.QualificationMaxAge > 0 && in.QualificationAge > in.QualificationMaxAge) {
		r.Blockers = append(r.Blockers, "synthetic lifecycle qualification stale")
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

const CanaryReadinessMaxAge = 30 * time.Minute

func ValidateCanaryReadinessReport(report CanaryReadinessResult, now time.Time) []string {
	reasons := []string{}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if report.Verdict != CanaryReady {
		reasons = append(reasons, "Hermes canary readiness verdict is not READY")
	}
	if report.GeneratedAt.IsZero() || report.GeneratedAt.After(now.Add(2*time.Minute)) || now.Sub(report.GeneratedAt) > CanaryReadinessMaxAge {
		reasons = append(reasons, "Hermes canary readiness report missing, future-dated, or stale")
	}
	if !report.QualificationPassed || report.StressRequired <= 0 || report.StressPassed < report.StressRequired {
		reasons = append(reasons, "Hermes lifecycle qualification incomplete")
	}
	if strings.TrimSpace(report.QualificationArtifact) == "" || strings.TrimSpace(report.QualificationSHA256) == "" {
		reasons = append(reasons, "Hermes lifecycle qualification provenance missing")
	}
	if report.DoctorStatus == DoctorBlock || report.DoctorStatus == "" {
		reasons = append(reasons, "Hermes canary runtime doctor not safe")
	}
	if report.ReconcileStatus != ReconcileClean {
		reasons = append(reasons, "Hermes canary reconcile not clean")
	}
	if report.OperatorHalted {
		reasons = append(reasons, "Hermes canary operator halt active")
	}
	if report.HermesDemoted {
		reasons = append(reasons, "Hermes canary circuit breaker demoted")
	}
	if !report.ExecutionAuthority {
		reasons = append(reasons, "Hermes canary execution authority disabled")
	}
	if report.OpenLiveOrders != 0 || report.HermesOwnedPositions != 0 {
		reasons = append(reasons, fmt.Sprintf("Hermes canary requires zero initial orders and positions; orders=%d positions=%d", report.OpenLiveOrders, report.HermesOwnedPositions))
	}
	return uniqueCanaryStrings(reasons)
}
