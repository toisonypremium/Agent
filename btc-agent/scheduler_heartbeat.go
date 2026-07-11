package main

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/reportio"
)

type SchedulerHeartbeat struct {
	GeneratedAt             string `json:"generated_at"`
	PID                     int    `json:"pid"`
	Status                  string `json:"status"`
	Timezone                string `json:"timezone"`
	Mode                    string `json:"mode"`
	DryRun                  bool   `json:"dry_run"`
	LiveEnabled             bool   `json:"live_enabled"`
	LiveSupervisorEnabled   bool   `json:"live_supervisor_enabled"`
	ResearchEnabled         bool   `json:"research_enabled"`
	MaintenanceEnabled      bool   `json:"maintenance_enabled"`
	NextDailyRun            string `json:"next_daily_run,omitempty"`
	NextMaintenanceRun      string `json:"next_maintenance_run,omitempty"`
	NextResearchBrief       string `json:"next_research_brief,omitempty"`
	NextMarketWatch         string `json:"next_market_watch,omitempty"`
	NextReconcile           string `json:"next_reconcile,omitempty"`
	NextLiveSupervisorCycle string `json:"next_live_supervisor_cycle,omitempty"`
	LastEvent               string `json:"last_event,omitempty"`
	LastEventAt             string `json:"last_event_at,omitempty"`
	DoctorStatus            string `json:"doctor_status,omitempty"`
	DoctorSummary           string `json:"doctor_summary,omitempty"`
	ConsecutiveDoctorBlocks int    `json:"consecutive_doctor_blocks"`
	ConsecutiveMarketErrors int    `json:"consecutive_market_errors"`
}

func writeSchedulerHeartbeat(h SchedulerHeartbeat) error {
	if h.GeneratedAt == "" {
		h.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return reportio.WriteJSON("reports", "scheduler_heartbeat_latest.json", h)
}

func schedulerHeartbeatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func buildAlivePingText(h SchedulerHeartbeat) string {
	var b strings.Builder
	b.WriteString("🟢 BTC Agent — Bot Alive\n")
	b.WriteString(fmt.Sprintf("PID: %d | Mode: %s | DryRun: %v\n", h.PID, emptyDefaultSched(h.Mode, "unset"), h.DryRun))
	b.WriteString(fmt.Sprintf("Status: %s | Doctor: %s\n", h.Status, emptyDefaultSched(h.DoctorStatus, "unknown")))
	if h.ConsecutiveDoctorBlocks > 0 {
		b.WriteString(fmt.Sprintf("⚠️ Consecutive doctor blocks: %d\n", h.ConsecutiveDoctorBlocks))
	}
	b.WriteString(fmt.Sprintf("Last event: %s @ %s\n", emptyDefaultSched(h.LastEvent, "—"), emptyDefaultSched(h.LastEventAt, "—")))
	if h.NextDailyRun != "" {
		b.WriteString(fmt.Sprintf("Next daily: %s\n", h.NextDailyRun))
	}
	if h.NextLiveSupervisorCycle != "" {
		b.WriteString(fmt.Sprintf("Next supervisor: %s\n", h.NextLiveSupervisorCycle))
	}
	if h.DoctorSummary != "" {
		b.WriteString(fmt.Sprintf("Doctor: %s\n", h.DoctorSummary))
	}
	b.WriteString("An toàn: spot limit BUY post-only; không futures, không leverage, không market order.\n")
	return b.String()
}

func emptyDefaultSched(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
