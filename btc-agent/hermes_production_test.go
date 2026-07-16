package main

import (
	"testing"

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
