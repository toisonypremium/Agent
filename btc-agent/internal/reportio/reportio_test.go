package reportio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeLabel(t *testing.T) {
	got := SafeLabel("../a/b\\c")
	if strings.Contains(got, "..") || strings.Contains(got, "/") || strings.Contains(got, "\\") {
		t.Fatalf("unsafe label: %q", got)
	}
	if SafeLabel("   ") != "telegram" {
		t.Fatal("empty label should use fallback")
	}
}

func TestWriteJSONAndMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := WriteJSON(dir, "x.json", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarkdown(dir, "x.md", "hello"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"x.json", "x.md"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatalf("%s permissions too open: %v", name, info.Mode().Perm())
		}
	}
}

func TestReadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.json")
	if err := WriteJSON(dir, "value.json", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := ReadJSON(path, &got); err != nil {
		t.Fatal(err)
	}
	if got["a"] != "b" {
		t.Fatalf("got %#v", got)
	}
	if err := ReadJSON(filepath.Join(dir, "missing.json"), &got); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing report error = %v, want os.ErrNotExist", err)
	}
}

func TestWriteReplacesExistingFileAtomically(t *testing.T) {
	dir := t.TempDir()
	if err := WriteMarkdown(dir, "report.md", "old"); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarkdown(dir, "report.md", "new"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "new" {
		t.Fatalf("content = %q, want new", b)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".tmp-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("orphan temp files: %v", matches)
	}
}
