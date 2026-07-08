package liveguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendShadowProbeJournalCappedNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	for i := 0; i < 10; i++ {
		line, _ := json.Marshal(map[string]int{"i": i})
		if err := appendShadowProbeJournalCapped(path, line, 200); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	data, _ := os.ReadFile(path)
	lines := nonEmptyLines(string(data))
	if len(lines) != 10 {
		t.Fatalf("want 10 lines, got %d", len(lines))
	}
}

func TestAppendShadowProbeJournalCappedTrimsOldest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	const max = 200
	// Write max+50 entries; only last max should survive.
	for i := 0; i < max+50; i++ {
		line, _ := json.Marshal(map[string]int{"i": i})
		if err := appendShadowProbeJournalCapped(path, line, max); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	data, _ := os.ReadFile(path)
	lines := nonEmptyLines(string(data))
	if len(lines) != max {
		t.Fatalf("want %d lines after cap, got %d", max, len(lines))
	}
	// Verify oldest entries are gone (first entry should have i >= 50).
	var first map[string]int
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first["i"] != 50 {
		t.Fatalf("want oldest surviving entry i=50, got i=%d", first["i"])
	}
}

func TestAppendShadowProbeJournalCappedExactlyAtCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	const max = 5
	for i := 0; i < max; i++ {
		line, _ := json.Marshal(map[string]int{"i": i})
		if err := appendShadowProbeJournalCapped(path, line, max); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	data, _ := os.ReadFile(path)
	lines := nonEmptyLines(string(data))
	if len(lines) != max {
		t.Fatalf("want %d lines, got %d", max, len(lines))
	}
}

func TestSaveAndLoadShadowProbeJournalCap(t *testing.T) {
	dir := t.TempDir()
	j := ShadowProbeJournal{
		Profile:              ShadowProfileArmedProbeLight,
		ProductionPermission: "WATCH",
		ResearchPermission:   "WATCH",
	}
	j.refreshSummary()
	// Write 210 entries — journal should stay at 200.
	for i := 0; i < 210; i++ {
		if err := SaveShadowProbeJournal(dir, j); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "shadow_probe_journal.jsonl"))
	lines := nonEmptyLines(string(data))
	if len(lines) != shadowProbeJournalMaxEntries {
		t.Fatalf("want %d entries, got %d", shadowProbeJournalMaxEntries, len(lines))
	}
	// latest.json must still be written.
	if _, err := os.Stat(filepath.Join(dir, "shadow_probe_latest.json")); err != nil {
		t.Fatal("shadow_probe_latest.json missing")
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
