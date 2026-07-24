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
	if body, err := os.ReadFile(heartbeatPath); err == nil {
		var h heartbeat
		if json.Unmarshal(body, &h) == nil {
			if at, e := time.Parse(time.RFC3339, h.GeneratedAt); e == nil {
				snapshot.HeartbeatAgeSeconds = int64(now.Sub(at.UTC()).Seconds())
				snapshot.HeartbeatState = h.Status
				if h.Status == "running" && h.PID > 0 {
					if _, statErr := os.Stat(filepath.Join("/proc", fmt.Sprintf("%d", h.PID))); statErr == nil {
						snapshot.SchedulerCount = 1
					}
				}
			}
		}
	}
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
	if err := os.MkdirAll(outDir, 0700); err != nil {
		log.Fatal(err)
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		log.Fatal(err)
	}
	tmp, err := os.CreateTemp(outDir, ".runtime-health-")
	if err != nil {
		log.Fatal(err)
	}
	if _, err = tmp.Write(body); err != nil {
		tmp.Close()
		log.Fatal(err)
	}
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		log.Fatal(err)
	}
	if err = tmp.Close(); err != nil {
		log.Fatal(err)
	}
	if err = os.Rename(tmp.Name(), filepath.Join(outDir, "web_console_runtime_health.json")); err != nil {
		log.Fatal(err)
	}
}
