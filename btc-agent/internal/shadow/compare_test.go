package shadow

import "testing"

func TestEquivalentInputsPass(t *testing.T) {
	v := map[string]any{"decision": map[string]any{"state": "WATCH"}, "plan": []any{1}, "allocation": []any{2}, "orders": []any{}, "reconciliation": "clean"}
	if !Pass(Compare("r1", v, v)) {
		t.Fatal("equivalent comparison failed")
	}
}
func TestMismatchFailsWithEvidence(t *testing.T) {
	a := map[string]any{"decision": "WATCH"}
	b := map[string]any{"decision": "ACTIVE_LIMIT"}
	c := Compare("r2", a, b)
	if Pass(c) || len(c.Mismatches) != 1 {
		t.Fatalf("bad mismatch=%+v", c)
	}
}
