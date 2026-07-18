package main

import (
	"testing"
	"time"

	"btc-agent/internal/hermesoperator"

	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
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
