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

type thesisRecorderStub struct {
	fakeManagedRecorder
	thesisCalls int
	legacyCalls int
}

func (r *thesisRecorderStub) ReserveManagedLiveOrder(clientOrderID string, desired ManagedDesiredOrder, reason string) error {
	r.legacyCalls++
	return r.fakeManagedRecorder.ReserveManagedLiveOrder(clientOrderID, desired, reason)
}

func (r *thesisRecorderStub) ReserveManagedLiveOrderWithThesis(clientOrderID string, desired ManagedDesiredOrder, reason string) error {
	r.thesisCalls++
	return nil
}

func TestReserveManagedOrderRoutesThesisBuyAtomically(t *testing.T) {
	r := &thesisRecorderStub{}
	buy := ManagedDesiredOrder{ThesisID: "thesis-1", Side: "BUY"}
	if err := reserveManagedOrder(r, "buy-1", buy, "test"); err != nil {
		t.Fatal(err)
	}
	if r.thesisCalls != 1 || r.legacyCalls != 0 {
		t.Fatalf("wrong route: thesis=%d legacy=%d", r.thesisCalls, r.legacyCalls)
	}
}

func TestReserveManagedOrderFailsClosedWithoutThesisCapability(t *testing.T) {
	r := &fakeManagedRecorder{}
	buy := ManagedDesiredOrder{ThesisID: "thesis-1", Side: "BUY"}
	if err := reserveManagedOrder(r, "buy-1", buy, "test"); err == nil {
		t.Fatal("expected thesis capability failure")
	}
	if len(r.reserved) != 0 {
		t.Fatalf("legacy reserve used for thesis BUY: %+v", r.reserved)
	}
}

func TestReserveManagedOrderKeepsLegacyAndSellPaths(t *testing.T) {
	r := &thesisRecorderStub{}
	if err := reserveManagedOrder(r, "legacy-buy", ManagedDesiredOrder{Side: "BUY"}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := reserveManagedOrder(r, "thesis-sell", ManagedDesiredOrder{ThesisID: "thesis-1", Side: "SELL"}, "test"); err != nil {
		t.Fatal(err)
	}
	if r.thesisCalls != 0 || r.legacyCalls != 2 {
		t.Fatalf("wrong route: thesis=%d legacy=%d", r.thesisCalls, r.legacyCalls)
	}
}
