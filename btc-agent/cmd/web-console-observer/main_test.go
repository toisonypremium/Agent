package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeartbeatSnapshotRequiresFreshLivePID(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "heartbeat.json")
	if err := os.WriteFile(path, []byte(`{"generated_at":"`+now.Format(time.RFC3339)+`","status":"running","pid":999999999}`), 0600); err != nil {
		t.Fatal(err)
	}
	got := readHeartbeat(path, now)
	if got.State != "running" || got.SchedulerCount != 0 {
		t.Fatalf("got=%+v", got)
	}
	if err := os.WriteFile(path, []byte(`{"generated_at":"`+now.Add(-6*time.Minute).Format(time.RFC3339)+`","status":"running","pid":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	got = readHeartbeat(path, now)
	if got.State != "stale" || got.SchedulerCount != 0 {
		t.Fatalf("stale=%+v", got)
	}
}

func TestWriteArtifactIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := writeArtifact(dir, []byte(`{"observer_state":"pass"}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "web_console_runtime_health.json"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("info=%v err=%v", info, err)
	}
}
