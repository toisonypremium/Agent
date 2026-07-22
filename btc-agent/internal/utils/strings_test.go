package utils_test

import (
	"testing"

	"btc-agent/internal/utils"
)

func TestFirstNonEmpty(t *testing.T) {
	if got := utils.FirstNonEmpty("", "b", "c"); got != "b" {
		t.Fatalf("want b got %q", got)
	}
	if got := utils.FirstNonEmpty("", "", ""); got != "" {
		t.Fatalf("want empty got %q", got)
	}
	if got := utils.FirstNonEmpty("a"); got != "a" {
		t.Fatalf("want a got %q", got)
	}
}

func TestUniqueStrings(t *testing.T) {
	got := utils.UniqueStrings([]string{"a", "b", "a", "c", "b"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("want [a b c] got %v", got)
	}
	got = utils.UniqueStrings(nil)
	if len(got) != 0 {
		t.Fatalf("want empty got %v", got)
	}
}
