package storage

import (
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyDoesNotBootstrapMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(path); err == nil {
		t.Fatal("missing database unexpectedly opened")
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := reader.SetHaltStatus(false); err == nil {
		t.Fatal("read-only database accepted mutation")
	}
}
