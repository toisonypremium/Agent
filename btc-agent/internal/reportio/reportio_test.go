package reportio

import (
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
