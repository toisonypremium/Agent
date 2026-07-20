package operatorcapability

import "testing"

func has(xs []Capability, w Capability) bool {
	for _, x := range xs {
		if x == w {
			return true
		}
	}
	return false
}
func TestEvaluateFailClosedAndNoOrderCapability(t *testing.T) {
	d := Evaluate(State{})
	if len(d.Capabilities) != 0 {
		t.Fatal(d)
	}
	for c := range d.Denied {
		if c == "PLACE_ORDER" || c == "SET_HERMES_AUTONOMOUS" {
			t.Fatal(c)
		}
	}
}
func TestIncreaseNeedsDistinctApproverConfiguration(t *testing.T) {
	s := State{Authenticated: true, Identity: "a@example.com", HeartbeatFresh: true, DoctorHealthy: true, ReconcileClean: true, Flags: Flags{IncreaseRisk: true}}
	if has(Evaluate(s).Capabilities, ProposeRiskIncrease) {
		t.Fatal("increase granted")
	}
	s.SecondApproverConfigured = true
	if !has(Evaluate(s).Capabilities, ProposeRiskIncrease) {
		t.Fatal("increase missing")
	}
}
