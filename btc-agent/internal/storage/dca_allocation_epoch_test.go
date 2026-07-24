package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCreateDCAAllocationEpochCreatesThreeImmutableAllocations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	epoch, created, err := db.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "bootstrap-1", ObservedAvailableUSDT: 2000, EnvelopeUSDT: 1600, NetNewUSDT: 1600, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)})
	if err != nil || !created {
		t.Fatalf("epoch=%+v created=%v err=%v", epoch, created, err)
	}
	if epoch.Allocations[0].ThesisID != "thesis-eth" || epoch.Allocations[0].AmountUSDT != 640 || epoch.Allocations[1].AmountUSDT != 560 || epoch.Allocations[2].AmountUSDT != 400 {
		t.Fatalf("allocations=%+v", epoch.Allocations)
	}
	if _, err := db.ThesisCapitalLedgerByID("thesis-eth"); err == nil {
		t.Fatal("allocation epoch must not create a thesis ledger")
	}
}

func TestCreateDCAAllocationEpochReplaysAndRejectsConflict(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := DCAAllocationEpochRequest{IdempotencyKey: "deposit-1", ObservedAvailableUSDT: 2100, EnvelopeUSDT: 1680, NetNewUSDT: 80, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)}
	first, created, err := db.CreateDCAAllocationEpoch(r)
	if err != nil || !created {
		t.Fatalf("%+v %v %v", first, created, err)
	}
	replay, created, err := db.CreateDCAAllocationEpoch(r)
	if err != nil || created || replay.ID != first.ID {
		t.Fatalf("%+v %v %v", replay, created, err)
	}
	r.NetNewUSDT = 81
	if _, _, err := db.CreateDCAAllocationEpoch(r); err == nil {
		t.Fatal("expected idempotency payload conflict")
	}
}

func TestDCAAllocationEpochRejectsInvalidFunding(t *testing.T) {
	for _, r := range []DCAAllocationEpochRequest{{}, {IdempotencyKey: "bad", ObservedAvailableUSDT: 100, EnvelopeUSDT: 81, NetNewUSDT: 80}, {IdempotencyKey: "bad", ObservedAvailableUSDT: 100, EnvelopeUSDT: 80, NetNewUSDT: -1}} {
		if err := ValidateDCAAllocationEpochRequest(r); err == nil {
			t.Fatalf("expected validation failure: %+v", r)
		}
	}
}

func TestApplyDCAAllocationEpochToThesesCreatesAndReplaysLedgers(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	epoch, _, err := db.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "bootstrap", ObservedAvailableUSDT: 2000, EnvelopeUSDT: 1600, NetNewUSDT: 1600, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := db.ApplyDCAAllocationEpochToTheses(epoch.ID)
	if err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	for id, want := range map[string]float64{"thesis-eth": 640, "thesis-link": 560, "thesis-virtual": 400} {
		got, err := db.ThesisCapitalLedgerByID(id)
		if err != nil || got.MaxExposureUSDT != want || got.RemainingDCAUSDT != want || got.Status != "ALLOCATED" {
			t.Fatalf("id=%s got=%+v err=%v", id, got, err)
		}
	}
	applied, err = db.ApplyDCAAllocationEpochToTheses(epoch.ID)
	if err != nil || applied {
		t.Fatalf("replay applied=%v err=%v", applied, err)
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-eth")
	if err != nil || got.MaxExposureUSDT != 640 {
		t.Fatalf("ledger=%+v err=%v", got, err)
	}
}

func TestApplyDCAAllocationEpochRejectsUnknownEpochAndConflictingLedger(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ApplyDCAAllocationEpochToTheses(999); err == nil {
		t.Fatal("expected missing epoch")
	}
	epoch, _, err := db.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "bootstrap", ObservedAvailableUSDT: 2000, EnvelopeUSDT: 1600, NetNewUSDT: 1600, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-eth", Symbol: "BTCUSDT", MaxExposureUSDT: 1, RemainingDCAUSDT: 1, Status: "ALLOCATED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApplyDCAAllocationEpochToTheses(epoch.ID); err == nil {
		t.Fatal("expected symbol conflict")
	}
	var applications int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dca_allocation_entry_applications`).Scan(&applications); err != nil || applications != 0 {
		t.Fatalf("applications=%d err=%v", applications, err)
	}
}

func TestLatestDCAAllocationEpoch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.LatestDCAAllocationEpoch(); err == nil {
		t.Fatal("expected missing epoch")
	}
	first, _, err := db.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "one", ObservedAvailableUSDT: 100, EnvelopeUSDT: 80, NetNewUSDT: 80, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := db.LatestDCAAllocationEpoch()
	if err != nil || latest.ID != first.ID {
		t.Fatalf("epoch=%+v err=%v", latest, err)
	}
}
