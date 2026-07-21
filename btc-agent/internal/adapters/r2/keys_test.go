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
	if got := LLMUsageKey(at, "request/../1"); got != "llm-usage/2026/07/20/events/request-..-1.json" {
		t.Fatalf("usage key=%q", got)
	}
	if got := LLMUsageDailyKey(at, "hash/1"); got != "llm-usage/2026/07/20/summary-hash-1.json" {
		t.Fatalf("daily usage key=%q", got)
	}
}
