package storage

import (
	"testing"
	"time"
)

func TestRuntimeEventsPendingHandleAndDedup(t *testing.T) {
	db, err := Open(t.TempDir() + "/events.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	event := RuntimeEvent{Timestamp: time.Unix(100, 0), Source: "market-watch", Type: "MARKET_STATE_CHANGED", Severity: "info", Fingerprint: "fp-1", PayloadJSON: `{"state":"SCOUT"}`}
	if err := db.SaveRuntimeEvent(event); err != nil {
		t.Fatalf("save event: %v", err)
	}
	if err := db.SaveRuntimeEvent(event); err != nil {
		t.Fatalf("save duplicate event: %v", err)
	}

	events, err := db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatalf("pending events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 deduped event, got %d", len(events))
	}
	if events[0].Source != "market-watch" || events[0].Type != "MARKET_STATE_CHANGED" || events[0].Fingerprint != "fp-1" {
		t.Fatalf("unexpected event: %+v", events[0])
	}

	if err := db.MarkRuntimeEventHandled(events[0].ID, time.Unix(200, 0)); err != nil {
		t.Fatalf("mark handled: %v", err)
	}
	events, err = db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatalf("pending events after handled: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no pending events, got %d", len(events))
	}
}

func TestMaintenancePrunesRuntimeEvents(t *testing.T) {
	db, err := Open(t.TempDir() + "/maintenance.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	old := time.Unix(100, 0)
	if err := db.SaveRuntimeEvent(RuntimeEvent{Timestamp: old, Source: "test", Type: "OLD", Severity: "info", Fingerprint: "old"}); err != nil {
		t.Fatalf("save old event: %v", err)
	}
	result, err := db.PruneMaintenance(MaintenanceConfig{EventRetentionDays: 1}, old.AddDate(0, 0, 2))
	if err != nil {
		t.Fatalf("maintenance: %v", err)
	}
	if result.RuntimeEventsDeleted != 1 {
		t.Fatalf("expected 1 runtime event pruned, got %d", result.RuntimeEventsDeleted)
	}
}
