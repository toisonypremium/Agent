// web-console-observer writes one fixed, sanitized health artifact for the
// separate Web Console. It has no scheduler, exchange, or mutation authority.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/storage"
	"btc-agent/internal/webconsole"
)

type heartbeatStatus struct {
	State          string
	AgeSeconds     int64
	SchedulerCount int
}

func readHeartbeat(path string, now time.Time) heartbeatStatus {
	out := heartbeatStatus{State: "unavailable"}
	body, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var h heartbeat
	if json.Unmarshal(body, &h) != nil {
		return out
	}
	at, err := time.Parse(time.RFC3339, h.GeneratedAt)
	if err != nil {
		return out
	}
	out.AgeSeconds = int64(now.Sub(at.UTC()).Seconds())
	if out.AgeSeconds < 0 || out.AgeSeconds > 300 {
		out.State = "stale"
		return out
	}
	out.State = h.Status
	if h.Status == "running" && h.PID > 0 {
		if _, err := os.Stat(filepath.Join("/proc", fmt.Sprintf("%d", h.PID))); err == nil {
			out.SchedulerCount = 1
		}
	}
	return out
}
func writeArtifact(outDir string, body []byte) error {
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(outDir, ".runtime-health-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(outDir, "web_console_runtime_health.json"))
}

type heartbeat struct {
	GeneratedAt string `json:"generated_at"`
	Status      string `json:"status"`
	PID         int    `json:"pid"`
}

func main() {
	dbPath, heartbeatPath, outDir := os.Getenv("BTC_AGENT_WEB_OBSERVER_DB_PATH"), os.Getenv("BTC_AGENT_WEB_OBSERVER_HEARTBEAT_PATH"), os.Getenv("BTC_AGENT_WEB_OBSERVER_OUTPUT_DIR")
	if dbPath == "" || heartbeatPath == "" || outDir == "" {
		log.Fatal("observer db, heartbeat and output paths required")
	}
	now := time.Now().UTC()
	snapshot := webconsole.RuntimeHealthSnapshot{ObservedAt: now, SchedulerCount: 0, HeartbeatState: "unavailable", DatabaseState: "unavailable", ObserverState: "fail"}
	h := readHeartbeat(heartbeatPath, now)
	snapshot.HeartbeatState, snapshot.HeartbeatAgeSeconds, snapshot.SchedulerCount = h.State, h.AgeSeconds, h.SchedulerCount
	db, err := storage.OpenReadOnly(dbPath)
	if err == nil {
		defer db.Close()
		if db.QueryRow("PRAGMA quick_check").Scan(new(string)) == nil {
			snapshot.DatabaseState = "ok"
		}
		if lease, ok, e := db.CurrentExecutionLease(context.Background(), "okx-live"); e == nil && ok {
			snapshot.LeaseInstanceID = lease.InstanceID
			snapshot.LeaseFresh = lease.ExpiresAt.After(now)
		}
	}
	if snapshot.HeartbeatState == "running" && snapshot.HeartbeatAgeSeconds >= 0 && snapshot.HeartbeatAgeSeconds <= 300 && snapshot.DatabaseState == "ok" && snapshot.LeaseFresh {
		snapshot.ObserverState = "pass"
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeArtifact(outDir, body); err != nil {
		log.Fatal(err)
	}
}
