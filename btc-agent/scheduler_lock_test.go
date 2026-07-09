package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAcquireSchedulerProcessLockCreatesAndReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler.lock")
	t.Setenv("BTC_AGENT_SCHEDULER_LOCK_FILE", path)
	release, err := acquireSchedulerProcessLock()
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock pid=%q want %d", strings.TrimSpace(string(b)), os.Getpid())
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file still exists after release: %v", err)
	}
}

func TestAcquireSchedulerProcessLockBlocksLivePID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler.lock")
	t.Setenv("BTC_AGENT_SCHEDULER_LOCK_FILE", path)
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := acquireSchedulerProcessLock()
	if err == nil {
		t.Fatal("expected lock acquire error for live pid")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcquireSchedulerProcessLockOverwritesStalePID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler.lock")
	t.Setenv("BTC_AGENT_SCHEDULER_LOCK_FILE", path)
	if err := os.WriteFile(path, []byte("99999999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	release, err := acquireSchedulerProcessLock()
	if err != nil {
		t.Fatalf("expected stale pid to be overwritten: %v", err)
	}
	defer release()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock pid=%q want current pid", strings.TrimSpace(string(b)))
	}
}
