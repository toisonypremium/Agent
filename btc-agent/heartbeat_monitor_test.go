package main

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateSchedulerHeartbeatHealthy(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	h := SchedulerHeartbeat{GeneratedAt: now.Add(-2 * time.Minute).Format(time.RFC3339), PID: 123, Status: "running", Mode: "live-auto", LastEvent: "scheduler ready"}
	check := evaluateSchedulerHeartbeat(h, true, 10*time.Minute, now)
	if check.Stale || check.State != "healthy" || check.AgeSeconds != 120 || check.Mode != "live-auto" {
		t.Fatalf("unexpected check: %+v", check)
	}
}

func TestEvaluateSchedulerHeartbeatStaleMissing(t *testing.T) {
	check := evaluateSchedulerHeartbeat(SchedulerHeartbeat{}, false, 10*time.Minute, time.Now().UTC())
	if !check.Stale || check.State != "stale" || !strings.Contains(check.Reason, "missing") {
		t.Fatalf("unexpected check: %+v", check)
	}
}

func TestEvaluateSchedulerHeartbeatStaleAge(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	h := SchedulerHeartbeat{GeneratedAt: now.Add(-11 * time.Minute).Format(time.RFC3339), Status: "running", Mode: "live-auto"}
	check := evaluateSchedulerHeartbeat(h, true, 10*time.Minute, now)
	if !check.Stale || check.State != "stale" || !strings.Contains(check.Reason, "exceeds") {
		t.Fatalf("unexpected check: %+v", check)
	}
}

func TestEvaluateSchedulerHeartbeatStoppedIsStale(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	h := SchedulerHeartbeat{GeneratedAt: now.Format(time.RFC3339), Status: "stopped", Mode: "live-auto"}
	check := evaluateSchedulerHeartbeat(h, true, 10*time.Minute, now)
	if !check.Stale || !strings.Contains(check.Reason, "stopped") {
		t.Fatalf("unexpected check: %+v", check)
	}
}

func TestSchedulerHeartbeatAlertTextIsSafe(t *testing.T) {
	check := SchedulerHeartbeatCheck{State: "stale", Stale: true, AgeSeconds: 999, MaxAgeSeconds: 600, Mode: "live-auto", SchedulerStatus: "running", LastEvent: "scheduler ready", Reason: "scheduler heartbeat age 999s exceeds max 600s"}
	text := schedulerHeartbeatAlertText(check)
	for _, want := range []string{"Scheduler heartbeat stale", "live-auto", "không đặt lệnh ngoài ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("alert text missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"TELEGRAM_TOKEN", "OKX", "secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("alert text contains forbidden %q:\n%s", forbidden, text)
		}
	}
}
