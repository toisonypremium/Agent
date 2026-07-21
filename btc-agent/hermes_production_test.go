package main

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

func TestNoActionHermesCycleIsSuccessfulDryRun(t *testing.T) {
	got := noActionHermesCycle(agent2.Plan{State: agent2.StateScout}, true, "valid hold")
	if got.Status != liveguard.ManagedCycleDryRun || got.Summary == "" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestInvalidHermesCycleRemainsBlocked(t *testing.T) {
	got := blockedHermesCycle(agent2.Plan{}, false, "invalid")
	if got.Status != liveguard.ManagedCycleBlocked {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSameHermesActionsDetectsPayloadMutation(t *testing.T) {
	a := []hermesoperator.Action{{Symbol: "BTCUSDT", Intent: hermesoperator.IntentProbeLimit, Confidence: .7, EntryPrice: 100, RequestedNotionalUSDT: 2}}
	b := append([]hermesoperator.Action(nil), a...)
	if !sameHermesActions(a, b) {
		t.Fatal("identical actions rejected")
	}
	b[0].RequestedNotionalUSDT = 3
	if sameHermesActions(a, b) {
		t.Fatal("mutated action accepted")
	}
}

func TestHermesExecutionTimeValidationRejectsExpiredDecision(t *testing.T) {
	now := time.Now().UTC()
	d := hermesoperator.Decision{Version: 1, DecisionID: "expired", GeneratedAt: now.Add(-3 * time.Minute), ValidUntil: now.Add(-time.Minute), Actions: []hermesoperator.Action{{Symbol: "BTCUSDT", Intent: hermesoperator.IntentProbeLimit, Confidence: .8, EntryPrice: 100, RequestedNotionalUSDT: 2}}}
	got := hermesoperator.Validate(d, hermesoperator.ValidationPolicy{Now: now, MaxDecisionTTL: 2 * time.Minute, MinConfidence: .6, MaxActions: 1, MaxProbeNotionalUSDT: 2, MaxActionNotionalUSDT: 2, AllowedSymbols: map[string]bool{"BTCUSDT": true}})
	if len(got.Reasons) == 0 || len(got.Actions) != 0 {
		t.Fatalf("expired decision executable: %+v", got)
	}
}

func TestPersistProtectionStatusesFailureCreatesCriticalEvent(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE hermes_runtime_state`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 21, 4, 35, 0, 0, time.UTC)
	if err := persistProtectionStatuses(db, []storage.ProtectionStatus{{Name: "loss_streak", Active: true}}, now); err == nil {
		t.Fatal("protection persistence failure must be returned")
	}
	events, err := db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "HERMES_PROTECTION_SNAPSHOT_FAILED" || events[0].Severity != "critical" {
		t.Fatalf("missing critical protection persistence event: %+v", events)
	}
}
