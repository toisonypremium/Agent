package hermesoperator

import (
	"encoding/json"
	"testing"
)

func TestActionThesisIDJSONRoundTrip(t *testing.T) {
	in := Action{ThesisID: "thesis-1", Symbol: "ETHUSDT", Intent: IntentProbeLimit}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Action
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.ThesisID != in.ThesisID {
		t.Fatalf("thesis id lost: %#v", out)
	}
}
