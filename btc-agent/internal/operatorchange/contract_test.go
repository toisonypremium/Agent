package operatorchange

import (
	"testing"
	"time"
)

func valid(a Action) Request {
	n := time.Now().UTC()
	return Request{ID: "change-123", Action: a, Requester: "first@example.com", Reason: "validated operator change", Before: map[string]float64{"cap": 10}, After: map[string]float64{"cap": 20}, SafetySnapshotID: "snapshot-123", SafetySnapshotSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: n, ExpiresAt: n.Add(10 * time.Minute), RequiredConfirmations: 2, Status: Pending}
}
func TestIncreaseDualControl(t *testing.T) {
	r := valid(IncreaseRiskCaps)
	if err := Validate(r, r.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(r, r.Requester); err == nil {
		t.Fatal("self approval")
	}
	r, err := Confirm(r, "second@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status == Confirmed {
		t.Fatal("confirmed early")
	}
	r, err = Confirm(r, "third@example.com")
	if err != nil || r.Status != Confirmed {
		t.Fatalf("%v %+v", err, r)
	}
}
func TestForbiddenActions(t *testing.T) {
	r := valid("PLACE_ORDER")
	if Validate(r, r.CreatedAt) == nil {
		t.Fatal("order accepted")
	}
	r.Action = "SET_HERMES_AUTONOMOUS"
	if Validate(r, r.CreatedAt) == nil {
		t.Fatal("autonomous accepted")
	}
}
func FuzzOperatorChange(f *testing.F) {
	f.Add("PLACE_ORDER", 1.0)
	f.Fuzz(func(t *testing.T, a string, v float64) {
		r := valid(Action(a))
		r.After["cap"] = v
		_ = Validate(r, r.CreatedAt)
	})
}
