package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneReportFilesDeletesOldUnprotectedFiles(t *testing.T) {
	dir := t.TempDir()
	old := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	writeReportFile(t, dir, "old.md", old)
	writeReportFile(t, dir, "newer.md", newer)
	writeReportFile(t, dir, "latest.md", old)
	writeReportFile(t, dir, "live_position_latest.json", old)

	deleted, err := PruneReportFiles(dir, 3, []string{"latest.md"})
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d want 1", deleted)
	}
	if exists(filepath.Join(dir, "old.md")) {
		t.Fatal("old unprotected file should be deleted")
	}
	if !exists(filepath.Join(dir, "newer.md")) || !exists(filepath.Join(dir, "latest.md")) || !exists(filepath.Join(dir, "live_position_latest.json")) {
		t.Fatal("protected or newer files should remain")
	}
}

func TestPruneReportFilesMissingDirNoop(t *testing.T) {
	deleted, err := PruneReportFiles(filepath.Join(t.TempDir(), "missing"), 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d want 0", deleted)
	}
}

func writeReportFile(t *testing.T, dir, name string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(name), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
