package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchedulerHeartbeatTime(t *testing.T) {
	if got := schedulerHeartbeatTime(time.Time{}); got != "" {
		t.Fatalf("zero time=%q want empty", got)
	}
	ts := time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC)
	if got := schedulerHeartbeatTime(ts); got != "2026-07-09T01:02:03Z" {
		t.Fatalf("time=%q", got)
	}
}

func TestWriteSchedulerHeartbeat(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	h := SchedulerHeartbeat{
		PID:                     123,
		Status:                  "running",
		Timezone:                "Asia/Ho_Chi_Minh",
		Mode:                    "live-auto",
		LiveEnabled:             true,
		LiveSupervisorEnabled:   true,
		NextDailyRun:            "2026-07-10T01:00:00Z",
		LastEvent:               "scheduler ready",
		DoctorStatus:            "DOCTOR_OK",
		ConsecutiveDoctorBlocks: 0,
	}
	if err := writeSchedulerHeartbeat(h); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("reports", "scheduler_heartbeat_latest.json"))
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	var got SchedulerHeartbeat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal heartbeat: %v", err)
	}
	if got.GeneratedAt == "" {
		t.Fatal("generated_at empty")
	}
	if got.PID != 123 || got.Status != "running" || got.LastEvent != "scheduler ready" || got.DoctorStatus != "DOCTOR_OK" {
		t.Fatalf("unexpected heartbeat: %+v", got)
	}
}
