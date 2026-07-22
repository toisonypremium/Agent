package circuitresearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerInvalidOutputDoesNotOverwriteLatest(t *testing.T) {
	input, _ := fixture(t)
	dir := t.TempDir()
	latest := filepath.Join(dir, "evidence_latest.json")
	if err := os.WriteFile(latest, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "bad.py")
	if err := os.WriteFile(script, []byte("import pathlib,sys; pathlib.Path(sys.argv[4]).write_text('{}')"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := RunDeterministic(context.Background(), input, RunnerConfig{Python: "/data/data/com.termux/files/usr/bin/python", Adapter: script, ProducerCommit: "sha", OutputDir: dir})
	if err == nil {
		t.Fatal("invalid output accepted")
	}
	raw, _ := os.ReadFile(latest)
	if string(raw) != "old" {
		t.Fatal("latest valid evidence overwritten")
	}
}

func TestRunnerTimeout(t *testing.T) {
	input, _ := fixture(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "slow.py")
	if err := os.WriteFile(script, []byte("import time; time.sleep(10)"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := RunDeterministic(context.Background(), input, RunnerConfig{Python: "/data/data/com.termux/files/usr/bin/python", Adapter: script, ProducerCommit: "sha", OutputDir: dir, Timeout: 20 * time.Millisecond})
	if err == nil {
		t.Fatal("timeout accepted")
	}
}
