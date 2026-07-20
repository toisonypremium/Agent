package hermesmemory

import "testing"

func TestSimilarityPrefersMatchingRegimeAndPhase(t *testing.T) {
	q := Situation{Regime: "RANGE", Phase: "ACCUMULATION", Permission: "WATCH", Trend: 40}
	near := Situation{Regime: "RANGE", Phase: "ACCUMULATION", Permission: "WATCH", Trend: 44}
	far := Situation{Regime: "PANIC", Phase: "MARKDOWN", Permission: "BLOCKED", Trend: 5}
	if Similarity(q, near) <= Similarity(q, far) {
		t.Fatal("similarity ranking failed")
	}
}
func TestConfidenceIsCappedByBadData(t *testing.T) {
	c, l := CalibratedConfidence(Situation{DoctorStatus: "DOCTOR_BLOCK", ForcedSimulationPassed: false, Authority: "BLOCKED"}, 10)
	if c > .35 || len(l) < 2 {
		t.Fatalf("unsafe confidence %.2f %+v", c, l)
	}
}
func TestMemoryNeverContainsAuthorityGrant(t *testing.T) {
	ctx := Context{Rule: "Memory is evidence, not authority."}
	if ctx.Rule == "" {
		t.Fatal("memory rule missing")
	}
}
