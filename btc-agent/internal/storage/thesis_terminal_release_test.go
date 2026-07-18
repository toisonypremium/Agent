package storage

import (
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestTerminalThesisReleaseAllAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "thesis-r", "order-r", 30)
	o := live.OrderStatus{ClientOrderID: "order-r", Status: live.StatusCancelled, UpdatedAt: 100}
	released, applied, err := db.SaveTerminalLiveOrderStatusAndRelease(o)
	if err != nil || !applied || released != 30 {
		t.Fatalf("release=%v applied=%v err=%v", released, applied, err)
	}
	released, applied, err = db.SaveTerminalLiveOrderStatusAndRelease(o)
	if err != nil || applied || released != 30 {
		t.Fatalf("replay release=%v applied=%v err=%v", released, applied, err)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-r")
	if err != nil || ledger.ReservedUSDT != 0 || ledger.RemainingDCAUSDT != 100 {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, applied, err = db.SaveTerminalLiveOrderStatusAndRelease(o)
	if err != nil || applied {
		t.Fatalf("restart replay applied=%v err=%v", applied, err)
	}
}

func TestTerminalThesisReleaseAfterPartialFill(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "partial-release.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-p", "order-p", 40)
	if applied, err := db.ApplyThesisBuyFillDelta("fill-p", "order-p", 15); err != nil || !applied {
		t.Fatalf("fill applied=%v err=%v", applied, err)
	}
	released, applied, err := db.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: "order-p", Status: live.StatusRejected})
	if err != nil || !applied || released != 25 {
		t.Fatalf("release=%v applied=%v err=%v", released, applied, err)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-p")
	if err != nil || ledger.ReservedUSDT != 0 || ledger.FilledUSDT != 15 || ledger.RemainingDCAUSDT != 85 {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
}

func TestTerminalReleaseRejectsUnknownAndRollsBackStatus(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "release-fail.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-u", "order-u", 10)
	if _, _, err := db.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: "order-u", Status: live.StatusUnknownNeedsManualCheck}); err == nil {
		t.Fatal("expected UNKNOWN rejection")
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM live_orders WHERE client_order_id='order-u'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != live.StatusPlanned {
		t.Fatalf("status changed=%s", status)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-u")
	if err != nil || ledger.ReservedUSDT != 10 {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
}

func TestTerminalReleaseLegacyOrderOnlyUpdatesStatus(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy-release.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveManagedLiveOrder("legacy-r", "", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, .1, 10, live.StatusPlanned, live.OrderStatus{}); err != nil {
		t.Fatal(err)
	}
	released, applied, err := db.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: "legacy-r", Status: live.StatusCancelled})
	if err != nil || !applied || released != 0 {
		t.Fatalf("release=%v applied=%v err=%v", released, applied, err)
	}
}

func TestMarkRejectedAndSaveCancelledUseAtomicRelease(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "central-release.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-reject", "reject-order", 10)
	if err := db.MarkManagedLiveOrderRejected("reject-order", "known reject"); err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "thesis-cancel", "cancel-order", 12)
	if err := db.SaveLiveOrderStatus(live.OrderStatus{ClientOrderID: "cancel-order", Status: live.StatusCancelled}); err != nil {
		t.Fatal(err)
	}
	for id, remaining := range map[string]float64{"thesis-reject": 100, "thesis-cancel": 100} {
		ledger, err := db.ThesisCapitalLedgerByID(id)
		if err != nil || ledger.ReservedUSDT != 0 || ledger.RemainingDCAUSDT != remaining {
			t.Fatalf("%s ledger=%+v err=%v", id, ledger, err)
		}
	}
}
