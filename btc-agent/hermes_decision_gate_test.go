package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

func TestHermesDecisionStateHashIgnoresTimestampAndTrigger(t *testing.T) {
	var cfg config.Config
	cfg.HermesOperator.Mode = "canary"
	a := hermesagent.HermesSnapshot{GeneratedAt: time.Now(), TriggerSource: "one", TriggerReason: "a", BTCPhase: "WATCH", Assets: []hermesagent.HermesAsset{{Symbol: "ETHUSDT", State: "SCOUT"}}}
	b := a
	b.GeneratedAt = a.GeneratedAt.Add(time.Hour)
	b.TriggerSource = "two"
	b.TriggerReason = "b"
	if hermesDecisionStateHash(cfg, a) != hermesDecisionStateHash(cfg, b) {
		t.Fatal("timestamp/trigger changed normalized state hash")
	}
	b.BTCPhase = "ACCUMULATION_CONFIRMED"
	if hermesDecisionStateHash(cfg, a) == hermesDecisionStateHash(cfg, b) {
		t.Fatal("material state change did not change hash")
	}
}

func TestHermesDecisionPreCallBlocksNonActionableState(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.MkdirAll("reports", 0700); err != nil {
		t.Fatal(err)
	}
	if err := saveJSONFile("reports", "live_reconcile_latest.json", liveguard.ReconcileResult{Safety: liveguard.ReconcileSafetyResult{Status: liveguard.ReconcileClean}}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHermesDemoted(false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.HermesOperator.Mode = "canary"
	snap := hermesagent.HermesSnapshot{GeneratedAt: time.Now(), DoctorStatus: "DOCTOR_OK", AuditVerdict: "APPROVED_REAL_ORDER"}
	got := evaluateHermesDecisionPreCall(cfg, db, snap)
	if got.Allowed || got.Reason != hermesGateNoActionable {
		t.Fatalf("got=%+v", got)
	}
}

func TestHermesDecisionPreCallBlocksUnownedExit(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.MkdirAll("reports", 0700); err != nil {
		t.Fatal(err)
	}
	if err := saveJSONFile("reports", "live_reconcile_latest.json", liveguard.ReconcileResult{Safety: liveguard.ReconcileSafetyResult{Status: liveguard.ReconcileClean}}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHermesDemoted(false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	snap := hermesagent.HermesSnapshot{GeneratedAt: time.Now(), DoctorStatus: "DOCTOR_OK", AuditVerdict: "APPROVED_REAL_ORDER", Exits: []hermesagent.HermesExit{{Symbol: "ETHUSDT", Action: "EXIT_LIMIT"}}}
	got := evaluateHermesDecisionPreCall(cfg, db, snap)
	if got.Allowed || got.Reason != hermesGateNoOwnedPosition {
		t.Fatalf("unowned exit must not call operator LLM: %+v", got)
	}
}

func TestHermesDecisionPreCallBlocksRemoteOnlyReconcile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.MkdirAll("reports", 0700); err != nil {
		t.Fatal(err)
	}
	unsafe := liveguard.ReconcileResult{Safety: liveguard.ReconcileSafetyResult{Status: liveguard.ReconcileBlock, RemoteOnly: 1}}
	if err := saveJSONFile("reports", "live_reconcile_latest.json", unsafe); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHermesDemoted(false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	snap := hermesagent.HermesSnapshot{GeneratedAt: time.Now(), DoctorStatus: "DOCTOR_OK", AuditVerdict: "APPROVED_REAL_ORDER", Assets: []hermesagent.HermesAsset{{Symbol: "ETHUSDT", ProbeEligible: true}}}
	got := evaluateHermesDecisionPreCall(cfg, db, snap)
	if got.Allowed || got.Reason != hermesGateReconcileBlock {
		t.Fatalf("got=%+v", got)
	}
}
