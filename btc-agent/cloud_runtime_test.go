package main

import (
	"btc-agent/internal/storage"
	"testing"
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
