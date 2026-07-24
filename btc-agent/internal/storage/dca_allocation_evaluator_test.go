package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDCAAllocationEvaluatorProposesBootstrapAfterFifteenMinutes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	if _, err := db.RecordVerifiedUSDTObservation(VerifiedUSDTObservation{ObservationKey: "a", AvailableUSDT: 2000, ObservedAt: at}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVerifiedUSDTObservation(VerifiedUSDTObservation{ObservationKey: "b", AvailableUSDT: 2000, ObservedAt: at.Add(15 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	proposal, err := db.EvaluateDCAAllocation(at.Add(15 * time.Minute))
	if err != nil || !proposal.Ready || proposal.Kind != DCAAllocationBootstrap || proposal.NetNewUSDT != 1600 || proposal.EnvelopeUSDT != 1600 {
		t.Fatalf("proposal=%+v err=%v", proposal, err)
	}
}

func TestDCAAllocationEvaluatorWaitsForStableDepositAndRejectsDecrease(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	_, _, err = db.CreateDCAAllocationEpoch(DCAAllocationEpochRequest{IdempotencyKey: "bootstrap", ObservedAvailableUSDT: 2000, EnvelopeUSDT: 1600, NetNewUSDT: 1600, ObservedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVerifiedUSDTObservation(VerifiedUSDTObservation{ObservationKey: "deposit", AvailableUSDT: 2100, ObservedAt: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	pending, err := db.EvaluateDCAAllocation(at.Add(10 * time.Minute))
	if err != nil || pending.Ready || pending.Reason != "funding_not_stable" {
		t.Fatalf("proposal=%+v err=%v", pending, err)
	}
	if _, err := db.RecordVerifiedUSDTObservation(VerifiedUSDTObservation{ObservationKey: "deposit-stable", AvailableUSDT: 2100, ObservedAt: at.Add(16 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	ready, err := db.EvaluateDCAAllocation(at.Add(16 * time.Minute))
	if err != nil || !ready.Ready || ready.Kind != DCAAllocationDeposit || ready.NetNewUSDT != 80 {
		t.Fatalf("proposal=%+v err=%v", ready, err)
	}
	if _, err := db.RecordVerifiedUSDTObservation(VerifiedUSDTObservation{ObservationKey: "decrease", AvailableUSDT: 1999, ObservedAt: at.Add(17 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	blocked, err := db.EvaluateDCAAllocation(at.Add(33 * time.Minute))
	if err != nil || blocked.Ready || blocked.Reason != "verified_usdt_decreased" {
		t.Fatalf("proposal=%+v err=%v", blocked, err)
	}
}

func TestVerifiedUSDTObservationReplayAndConflict(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "allocation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	o := VerifiedUSDTObservation{ObservationKey: "same", AvailableUSDT: 100, ObservedAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)}
	if _, err := db.RecordVerifiedUSDTObservation(o); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RecordVerifiedUSDTObservation(o); err != nil {
		t.Fatal(err)
	}
	o.AvailableUSDT = 101
	if _, err := db.RecordVerifiedUSDTObservation(o); err == nil {
		t.Fatal("expected conflict")
	}
}
