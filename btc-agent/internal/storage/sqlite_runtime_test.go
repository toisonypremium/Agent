package storage

import (
	"context"
	"testing"
	"time"

	"btc-agent/internal/runtime/outbox"
)

func TestSQLiteExecutionLeaseFencing(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	a, ok, err := db.AcquireExecutionLease(ctx, "okx", "a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if _, ok, err = db.AcquireExecutionLease(ctx, "okx", "b", now, time.Minute); err != nil || ok {
		t.Fatalf("second: ok=%v err=%v", ok, err)
	}
	b, ok, err := db.AcquireExecutionLease(ctx, "okx", "b", now.Add(2*time.Minute), time.Minute)
	if err != nil || !ok {
		t.Fatalf("expired: ok=%v err=%v", ok, err)
	}
	if b.FencingToken <= a.FencingToken {
		t.Fatalf("fence %d <= %d", b.FencingToken, a.FencingToken)
	}
}

func TestSQLiteOutboxIdempotencyClaimAndRetry(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	i := outbox.Item{ID: "event-1", EventType: "decision", Destination: "supabase", Payload: []byte(`{"ok":true}`), IdempotencyKey: "decision-1", CreatedAt: now}
	if err = db.EnqueueOutbox(ctx, i); err != nil {
		t.Fatal(err)
	}
	i.ID = "event-2"
	if err = db.EnqueueOutbox(ctx, i); err == nil {
		t.Fatal("duplicate idempotency key accepted")
	}
	items, err := db.ClaimOutbox(ctx, now, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("claim=%d err=%v", len(items), err)
	}
	if err = db.RetryOutbox(ctx, items[0].ID, "down", now.Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	items, err = db.ClaimOutbox(ctx, now, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("early retry claim=%d err=%v", len(items), err)
	}
	items, err = db.ClaimOutbox(ctx, now.Add(time.Minute), 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("due retry claim=%d err=%v", len(items), err)
	}
	if err = db.MarkOutboxDelivered(ctx, items[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteOutboxProcessingIsRecoverable(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	i := outbox.Item{ID: "crash-1", EventType: "heartbeat", Destination: "supabase", Payload: []byte("{}"), IdempotencyKey: "crash-1", CreatedAt: now}
	if err = db.EnqueueOutbox(ctx, i); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.ClaimOutbox(ctx, now, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v %v", claimed, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE outbox_events SET status=? WHERE id=?`, outbox.StatusProcessing, i.ID); err != nil {
		t.Fatal(err)
	}
	active, err := db.ClaimOutbox(ctx, now.Add(4*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active processing row must not be stolen: %v", active)
	}
	recovered, err := db.ClaimOutbox(ctx, now.Add(6*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 {
		t.Fatalf("stale processing row should recover: %v", recovered)
	}
}

func TestSQLiteLeaseFenceSurvivesGracefulRelease(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	first, ok, err := db.AcquireExecutionLease(ctx, "okx", "instance", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first ok=%v err=%v", ok, err)
	}
	if err = db.ReleaseExecutionLease(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, ok, err := db.AcquireExecutionLease(ctx, "okx", "instance", now.Add(time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("second ok=%v err=%v", ok, err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("fence did not advance across release: %d <= %d", second.FencingToken, first.FencingToken)
	}
}
