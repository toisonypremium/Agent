package r2

import (
	"testing"
	"time"
)

func TestDeterministicSafeKeys(t *testing.T) {
	at := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	got := ReportKey(at, "daily/../../secret", "json")
	want := "reports/2026/07/20/daily-..-..-secret.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got != ReportKey(at, "daily/../../secret", "json") {
		t.Fatal("key not deterministic")
	}
}
