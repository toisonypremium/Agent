package storage

import (
	"strings"
	"testing"
	"time"
)

func TestLLMUsagePersistsUnknownAndAggregatesMeasuredTokens(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	from := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	events := []LLMUsageEvent{
		{RequestID: "r1", Timestamp: from.Add(time.Hour), Purpose: "operator_decision", Model: "m", PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14, UsageAvailable: true, LatencyMS: 20, Status: "ok", StateHash: "same"},
		{RequestID: "r2", Timestamp: from.Add(2 * time.Hour), Purpose: "operator_decision", Model: "m", UsageAvailable: false, LatencyMS: 30, Status: "error", ErrorClass: "request", StateHash: "same"},
		{RequestID: "r3", Timestamp: from.Add(3 * time.Hour), Purpose: "operator_decision", Model: "m", UsageAvailable: false, Status: "skipped", ErrorClass: "NO_ACTIONABLE_CANDIDATE", StateHash: "other"},
	}
	for _, event := range events {
		if err := db.SaveLLMUsageEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveLLMUsageEvent(events[0]); err == nil {
		t.Fatal("duplicate request ID accepted")
	}
	var prompt any
	if err := db.QueryRow(`SELECT prompt_tokens FROM llm_usage_events WHERE request_id='r2'`).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if prompt != nil {
		t.Fatalf("missing provider usage must persist NULL, got %v", prompt)
	}
	summary, err := db.LLMUsageSummaryBetween(from, from.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Calls != 2 || summary.SkippedCalls != 1 || summary.FailedCalls != 1 || summary.UsageUnavailable != 1 || summary.TotalTokens != 14 || summary.RepeatedHashes != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestLLMUsageSchemaContainsNoPromptOrResponseColumns(t *testing.T) {
	db, err := Open(t.TempDir() + "/agent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(llm_usage_events)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "prompt_text") || strings.Contains(lower, "response") || strings.Contains(lower, "secret") {
			t.Fatalf("unsafe telemetry column %s", name)
		}
	}
}
