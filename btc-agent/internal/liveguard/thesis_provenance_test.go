package liveguard

import (
	"encoding/json"
	"testing"
)

func TestManagedDesiredOrderThesisIDJSONRoundTrip(t *testing.T) {
	in := ManagedDesiredOrder{ThesisID: "thesis-1", Symbol: "ETHUSDT", LayerIndex: 1}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ManagedDesiredOrder
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.ThesisID != in.ThesisID {
		t.Fatalf("thesis id lost: %#v", out)
	}
}
