package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/reportio"
)

type SchedulerHeartbeatCheck struct {
	GeneratedAt     time.Time `json:"generated_at"`
	State           string    `json:"state"`
	Stale           bool      `json:"stale"`
	Reason          string    `json:"reason"`
	AgeSeconds      int64     `json:"age_seconds,omitempty"`
	MaxAgeSeconds   int64     `json:"max_age_seconds"`
	HeartbeatAt     string    `json:"heartbeat_at,omitempty"`
	PID             int       `json:"pid,omitempty"`
	Mode            string    `json:"mode,omitempty"`
	DryRun          bool      `json:"dry_run"`
	SchedulerStatus string    `json:"scheduler_status,omitempty"`
	LastEvent       string    `json:"last_event,omitempty"`
}

type SchedulerHeartbeatAlertState struct {
	GeneratedAt time.Time `json:"generated_at"`
	State       string    `json:"state"`
	HeartbeatAt string    `json:"heartbeat_at,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

func runSchedulerHeartbeatCheck(ctx context.Context, cfg config.Config, maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = 10 * time.Minute
	}
	check := evaluateLatestSchedulerHeartbeat(maxAge, time.Now().UTC())
	if err := reportio.WriteJSON("reports", "scheduler_heartbeat_check_latest.json", check); err != nil {
		return err
	}
	fmt.Print(schedulerHeartbeatCheckText(check))
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		if check.Stale && shouldSendSchedulerHeartbeatAlert(check) {
			sendTelegram(ctx, cfg, "scheduler-heartbeat-stale", schedulerHeartbeatAlertText(check))
		}
		if err := saveSchedulerHeartbeatAlertState(check); err != nil {
			return err
		}
	}
	return nil
}

func evaluateLatestSchedulerHeartbeat(maxAge time.Duration, now time.Time) SchedulerHeartbeatCheck {
	heartbeat, ok := loadSchedulerHeartbeatReport()
	return evaluateSchedulerHeartbeat(heartbeat, ok, maxAge, now)
}

func evaluateSchedulerHeartbeat(h SchedulerHeartbeat, ok bool, maxAge time.Duration, now time.Time) SchedulerHeartbeatCheck {
	check := SchedulerHeartbeatCheck{GeneratedAt: now, MaxAgeSeconds: int64(maxAge.Seconds())}
	if !ok {
		check.Stale = true
		check.State = "stale"
		check.Reason = "missing scheduler heartbeat"
		return check
	}
	check.PID = h.PID
	check.Mode = h.Mode
	check.DryRun = h.DryRun
	check.SchedulerStatus = h.Status
	check.LastEvent = h.LastEvent
	heartbeatAt, heartbeatText, err := schedulerHeartbeatTimestamp(h)
	if err != nil {
		check.Stale = true
		check.State = "stale"
		check.Reason = err.Error()
		return check
	}
	check.HeartbeatAt = heartbeatText
	age := now.Sub(heartbeatAt)
	if age < 0 {
		age = 0
	}
	check.AgeSeconds = int64(age.Seconds())
	if h.Status != "running" && h.Status != "starting" {
		check.Stale = true
		check.State = "stale"
		check.Reason = "scheduler status is " + emptyStringDefault(h.Status, "unknown")
		return check
	}
	if age > maxAge {
		check.Stale = true
		check.State = "stale"
		check.Reason = fmt.Sprintf("scheduler heartbeat age %s exceeds max %s", age.Round(time.Second), maxAge.Round(time.Second))
		return check
	}
	check.State = "healthy"
	check.Reason = "scheduler heartbeat fresh"
	return check
}

func schedulerHeartbeatTimestamp(h SchedulerHeartbeat) (time.Time, string, error) {
	for _, value := range []string{h.GeneratedAt, h.LastEventAt} {
		if value == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, value)
		if err == nil {
			return ts.UTC(), value, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("scheduler heartbeat timestamp invalid")
}

func schedulerHeartbeatCheckText(check SchedulerHeartbeatCheck) string {
	return fmt.Sprintf("Scheduler heartbeat: %s | stale=%v | age=%ds | max=%ds | mode=%s | dry_run=%v | status=%s | reason=%s\n", check.State, check.Stale, check.AgeSeconds, check.MaxAgeSeconds, emptyStringDefault(check.Mode, "unknown"), check.DryRun, emptyStringDefault(check.SchedulerStatus, "unknown"), check.Reason)
}

func schedulerHeartbeatAlertText(check SchedulerHeartbeatCheck) string {
	return fmt.Sprintf("🔴 BTC Agent — Scheduler heartbeat stale\nState: %s | age=%ds max=%ds\nMode: %s | dry_run=%v | status=%s\nLast event: %s\nReason: %s\nAn toàn: bot phải fail-closed nếu scheduler chết; không đặt lệnh ngoài ACTIVE_LIMIT.\n", check.State, check.AgeSeconds, check.MaxAgeSeconds, emptyStringDefault(check.Mode, "unknown"), check.DryRun, emptyStringDefault(check.SchedulerStatus, "unknown"), emptyStringDefault(check.LastEvent, "unknown"), check.Reason)
}

func shouldSendSchedulerHeartbeatAlert(check SchedulerHeartbeatCheck) bool {
	prev, ok := loadSchedulerHeartbeatAlertState()
	if !ok {
		return true
	}
	return prev.State != check.State || prev.HeartbeatAt != check.HeartbeatAt || prev.Reason != check.Reason
}

func saveSchedulerHeartbeatAlertState(check SchedulerHeartbeatCheck) error {
	state := SchedulerHeartbeatAlertState{GeneratedAt: check.GeneratedAt, State: check.State, HeartbeatAt: check.HeartbeatAt, Reason: check.Reason}
	return reportio.WriteJSON("reports", "scheduler_heartbeat_alert_state.json", state)
}

func loadSchedulerHeartbeatAlertState() (SchedulerHeartbeatAlertState, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "scheduler_heartbeat_alert_state.json"))
	if err != nil {
		return SchedulerHeartbeatAlertState{}, false
	}
	var state SchedulerHeartbeatAlertState
	if err := json.Unmarshal(b, &state); err != nil {
		return SchedulerHeartbeatAlertState{}, false
	}
	return state, true
}
