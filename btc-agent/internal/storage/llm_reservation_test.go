package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLLMCallReservationDeduplicatesConcurrentState(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	var accepted int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, _, err := db.ReserveLLMCall(context.Background(), "operator_decision", "hash", now, time.Minute)
			if err != nil {
				t.Errorf("reserve: %v", err)
				return
			}
			if ok {
				atomic.AddInt32(&accepted, 1)
			}
		}()
	}
	wg.Wait()
	if accepted != 1 {
		t.Fatalf("accepted=%d", accepted)
	}
}

func TestLLMCallReservationFreshnessCooldownAndRecovery(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	if _, ok, _, err := db.ReserveLLMCall(ctx, "p", "success", now, time.Minute); err != nil || !ok {
		t.Fatalf("reserve ok=%v err=%v", ok, err)
	}
	if err := db.CompleteLLMCall(ctx, "p", "success", LLMReservationSuccess, "", now, time.Time{}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, ok, reason, err := db.ReserveLLMCall(ctx, "p", "success", now.Add(time.Minute), time.Minute); err != nil || ok || reason != "DECISION_STILL_FRESH" {
		t.Fatalf("fresh ok=%v reason=%s err=%v", ok, reason, err)
	}
	if _, ok, _, err := db.ReserveLLMCall(ctx, "p", "error", now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if err := db.CompleteLLMCall(ctx, "p", "error", LLMReservationError, "request", now, now.Add(2*time.Minute), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, ok, reason, _ := db.ReserveLLMCall(ctx, "p", "error", now.Add(time.Minute), time.Minute); ok || reason != "ERROR_COOLDOWN" {
		t.Fatalf("cooldown ok=%v reason=%s", ok, reason)
	}
	if _, ok, _, err := db.ReserveLLMCall(ctx, "p", "stale", now, time.Minute); err != nil || !ok {
		t.Fatal(err)
	}
	if _, ok, reason, err := db.ReserveLLMCall(ctx, "p", "stale", now.Add(2*time.Minute), time.Minute); err != nil || !ok || reason != "" {
		t.Fatalf("stale recovery ok=%v reason=%s err=%v", ok, reason, err)
	}
}
