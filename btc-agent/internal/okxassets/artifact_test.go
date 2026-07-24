package okxassets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteArtifactIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	if err := WriteArtifact(dir, Snapshot{Source: SourceOKXSpotReadOnly, Assets: []Asset{}}, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ArtifactFilename)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%#o", info.Mode().Perm())
	}
	got, err := LoadArtifact(dir, now.Add(time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateVerified || got.ObservedAt != now.Format(time.RFC3339) {
		t.Fatalf("snapshot=%+v", got)
	}
}
func TestLoadArtifactFailsClosedWhenStaleOrSymlink(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	if err := WriteArtifact(dir, Snapshot{Source: SourceOKXSpotReadOnly}, now); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadArtifact(dir, now.Add(6*time.Minute), 5*time.Minute); err != nil || got.State != StateStale {
		t.Fatalf("snapshot=%+v err=%v", got, err)
	}
	if err := os.Remove(filepath.Join(dir, ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other", filepath.Join(dir, ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadArtifact(dir, now, 5*time.Minute); err == nil {
		t.Fatal("symlink accepted")
	}
}
