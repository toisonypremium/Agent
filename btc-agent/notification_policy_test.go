package main

import "testing"

func TestAutoTelegramNotificationPolicy(t *testing.T) {
	allowed := []string{
		"expert-report",
		"market-critical",
		"market-watch-error",
		"scheduler-heartbeat-stale",
		"operator-halt",
		"operator-resume",
		"reconcile-live-orders",
		"auto-live-management",
		"manual-live-order",
		"cancel-all-live-orders",
	}
	for _, label := range allowed {
		if !shouldAutoSendTelegram(label) {
			t.Fatalf("critical label %q suppressed", label)
		}
	}
	suppressed := []string{
		"run-ai-watch",
		"research-brief",
		"hermes-cycle",
		"run-daily",
		"scheduler-run-now",
		"scheduler-alive",
		"market-state",
		"live-auto-near-unlock",
		"near-trigger",
		"live-supervisor",
	}
	for _, label := range suppressed {
		if shouldAutoSendTelegram(label) {
			t.Fatalf("legacy label %q allowed", label)
		}
	}
}
