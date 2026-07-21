package storage

import (
	"context"
	"testing"
	"time"

	"btc-agent/internal/runtime/outbox"
)

func TestDashboardSafetyQueriesExposeLeaseAndOutboxHealth(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if _, ok, err := db.AcquireExecutionLease(ctx, "okx-live", "instance", now, time.Minute); err != nil || !ok {
		t.Fatalf("lease ok=%v err=%v", ok, err)
	}
	lease, err := db.ExecutionLeaseForDashboard(ctx, "okx-live")
	if err != nil || lease["present"] != true || lease["fencing_token"] == nil {
		t.Fatalf("lease=%+v err=%v", lease, err)
	}
	item := outbox.Item{ID: "dead", EventType: "test", Destination: "r2", Payload: []byte("{}"), IdempotencyKey: "dead", Status: outbox.StatusDeadLetter, CreatedAt: now}
	if err := db.EnqueueOutbox(ctx, item); err != nil {
		t.Fatal(err)
	}
	health, err := db.OutboxHealth()
	if err != nil || health.DeadLetter != 1 {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}
