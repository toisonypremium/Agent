package main

import (
	"context"
	"testing"
	"time"

	"btc-agent/internal/storage"
)

func TestEnqueueLLMUsageCloudIsIdempotent(t *testing.T) {
	db, err := storage.Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, key := range []string{"SUPABASE_URL", "SUPABASE_SERVICE_ROLE_KEY", "R2_PRESIGNED_PUT_URL", "R2_ENDPOINT", "R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		t.Setenv(key, "")
	}
	t.Setenv("SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "test")
	event := storage.LLMUsageEvent{RequestID: "request-1", Timestamp: time.Now().UTC(), Purpose: "test", Model: "m", Status: "ok"}
	enqueueLLMUsageCloud(context.Background(), db, event)
	enqueueLLMUsageCloud(context.Background(), db, event)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE event_type='llm_usage'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one idempotent cloud event, got %d", count)
	}
}
