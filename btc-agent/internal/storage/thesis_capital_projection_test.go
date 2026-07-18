package storage

import (
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestThesisCapitalProjectionAuditHealthyAfterFillAndRelease(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "projection.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-proj", "order-proj", 40)
	if _, err := db.ApplyThesisBuyFillDelta("proj-fill", "order-proj", 15); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.SaveTerminalLiveOrderStatusAndRelease(live.OrderStatus{ClientOrderID: "order-proj", Status: live.StatusRejected}); err != nil {
		t.Fatal(err)
	}
	p, err := db.ThesisCapitalProjectionAudit("thesis-proj")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Healthy() {
		t.Fatalf("projection unhealthy: %+v", p)
	}
}

func TestThesisCapitalProjectionAuditDetectsDrift(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "projection-drift.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-drift", "order-drift", 20)
	if _, err := db.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=19 WHERE thesis_id='thesis-drift'`); err != nil {
		t.Fatal(err)
	}
	p, err := db.ThesisCapitalProjectionAudit("thesis-drift")
	if err != nil {
		t.Fatal(err)
	}
	if p.Healthy() {
		t.Fatalf("expected drift: %+v", p)
	}
}
