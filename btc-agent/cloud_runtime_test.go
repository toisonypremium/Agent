package main

import (
	"context"
	"testing"
	"time"

	"btc-agent/internal/storage"
)

func TestCloudRuntimeConfiguration(t *testing.T) {
	db, err := storage.Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, k := range []string{"SUPABASE_URL", "SUPABASE_SERVICE_ROLE_KEY", "R2_PRESIGNED_PUT_URL", "R2_ENDPOINT", "R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		t.Setenv(k, "")
	}
	c, err := newCloudRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.destinations) != 0 {
		t.Fatalf("expected disabled cloud, got %v", c.destinations)
	}
	t.Setenv("SUPABASE_URL", "https://example.supabase.co")
	if _, err = newCloudRuntime(db); err == nil {
		t.Fatal("expected partial Supabase config error")
	}
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "secret")
	t.Setenv("R2_ENDPOINT", "https://example.r2.cloudflarestorage.com")
	if _, err = newCloudRuntime(db); err == nil {
		t.Fatal("expected partial R2 config error")
	}
	t.Setenv("R2_BUCKET", "bucket")
	t.Setenv("R2_ACCESS_KEY_ID", "access")
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret")
	if _, err = newCloudRuntime(db); err != nil {
		t.Fatal(err)
	}
}

func TestCloudRuntimeEnqueuesIdempotentLLMUsageDailyArtifact(t *testing.T) {
	db, err := storage.Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	if err := db.SaveLLMUsageEvent(storage.LLMUsageEvent{RequestID: "r1", Timestamp: now, Purpose: "test", Model: "m", PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3, UsageAvailable: true, Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	cloud := &cloudRuntime{destinations: []string{"r2"}, db: db}
	cloud.enqueueLLMUsageDaily(context.Background(), now)
	cloud.enqueueLLMUsageDaily(context.Background(), now)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE event_type='llm_usage_daily'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("daily artifacts=%d", count)
	}
}
