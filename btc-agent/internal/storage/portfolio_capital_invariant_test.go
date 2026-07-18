package storage

import (
	"path/filepath"
	"testing"
)

func TestPortfolioCapitalInvariantAuditHealthyAndNoNestedQueryDeadlock(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "portfolio.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "portfolio-a", "portfolio-order", 20)
	r, err := db.PortfolioCapitalInvariantAudit(100)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Healthy || r.ThesisCount != 1 || r.ReservedUSDT != 20 || r.ActiveOrderNetReservedUSDT != 20 {
		t.Fatalf("report=%+v", r)
	}
}
func TestPortfolioCapitalInvariantAuditDetectsEnvelopeAndDrift(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "portfolio-drift.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "portfolio-b", "portfolio-order-b", 20)
	if _, err := db.Exec(`UPDATE thesis_capital_ledgers SET reserved_usdt=19 WHERE thesis_id='portfolio-b'`); err != nil {
		t.Fatal(err)
	}
	r, err := db.PortfolioCapitalInvariantAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.Healthy || r.DriftedThesisCount != 1 || r.EnvelopeExcessUSDT <= 0 {
		t.Fatalf("report=%+v", r)
	}
}
func TestPortfolioCapitalInvariantAuditDetectsOrphans(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "portfolio-orphan.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO live_orders(client_order_id,symbol,side,status,thesis_id) VALUES('orphan','ETHUSDT','BUY','PLANNED','missing')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO thesis_capital_events(event_key,thesis_id,client_order_id,event_type,notional_usdt,created_at,payload_json) VALUES('orphan-event','missing','orphan','BUY_FILL',1,1,'{}')`); err != nil {
		t.Fatal(err)
	}
	r, err := db.PortfolioCapitalInvariantAudit(100)
	if err != nil {
		t.Fatal(err)
	}
	if r.Healthy || r.OrphanThesisOrderCount != 1 || r.OrphanCapitalEventCount != 1 {
		t.Fatalf("report=%+v", r)
	}
}
func TestPortfolioCapitalInvariantAuditRejectsInvalidEnvelope(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "portfolio-invalid.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.PortfolioCapitalInvariantAudit(-1); err == nil {
		t.Fatal("negative envelope accepted")
	}
}

func TestPortfolioCapitalInvariantAuditEnforcesZeroEnvelope(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "portfolio-zero.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "portfolio-zero", "portfolio-zero-order", 20)
	r, err := db.PortfolioCapitalInvariantAudit(0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Healthy || r.EnvelopeExcessUSDT <= 0 {
		t.Fatalf("report=%+v", r)
	}
}
