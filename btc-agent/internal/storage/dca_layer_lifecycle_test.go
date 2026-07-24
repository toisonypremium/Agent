package storage

import (
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"path/filepath"
	"testing"
	"time"
)

func TestDCAReservationRequiresPriorKnownTerminalLayer(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "d.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	epoch, _, e := d.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "epoch", ObservedAvailableUSDT: 1000, EnvelopeUSDT: 800, NetNewUSDT: 800, ObservedAt: at})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = d.ApplyDCAAllocationEpochToTheses(epoch.ID); e != nil {
		t.Fatal(e)
	}
	if e = d.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-eth", Symbol: "ETHUSDT", MaxExposureUSDT: 100, RemainingDCAUSDT: 100, Status: "ALLOCATED"}); e != nil {
		t.Fatal(e)
	}
	l2 := liveguard.ManagedDesiredOrder{ThesisID: "thesis-eth", Symbol: "ETHUSDT", InstID: "ETH-USDT", Side: "BUY", Type: "limit", Price: 100, Quantity: .2, Notional: 20, LayerIndex: 2, Source: "dca_layer_2"}
	if e = d.ReserveManagedLiveOrderWithThesis("l2", l2, "test"); e == nil {
		t.Fatal("layer two without prior terminal must fail")
	}
	l1 := l2
	l1.LayerIndex = 1
	l1.Source = "dca_canary_layer_1"
	if e = d.ReserveManagedLiveOrderWithThesis("l1", l1, "test"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = d.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: "l1", Status: live.StatusCancelled}); e != nil {
		t.Fatal(e)
	}
	if e = d.ReserveManagedLiveOrderWithThesis("l2", l2, "test"); e != nil {
		t.Fatal(e)
	}
}
