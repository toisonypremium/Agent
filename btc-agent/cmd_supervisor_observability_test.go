package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/storage"
)

func TestRecordSupervisorOperationalFailurePersistsDeduplicatedWarning(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "supervisor.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	recordSupervisorOperationalFailure(db, now, "capital_utilization", errors.New("write failed"))
	recordSupervisorOperationalFailure(db, now.Add(time.Minute), "capital_utilization", errors.New("write failed again"))

	events, err := db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	event := events[0]
	if event.Source != "btc-agent-supervisor" || event.Type != "SUPERVISOR_OPERATIONAL_PERSISTENCE_FAILED" || event.Severity != "warning" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if !strings.Contains(event.PayloadJSON, "capital_utilization") || !strings.Contains(event.PayloadJSON, "write failed") {
		t.Fatalf("unexpected payload: %s", event.PayloadJSON)
	}
}

func TestRecordSupervisorOperationalFailureAllowsNilInputs(t *testing.T) {
	recordSupervisorOperationalFailure(nil, time.Now(), "ignored", errors.New("ignored"))
}
