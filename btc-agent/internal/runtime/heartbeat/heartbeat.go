package heartbeat

import "time"

type Status struct {
	GeneratedAt         time.Time `json:"generated_at"`
	InstanceID          string    `json:"instance_id"`
	GitSHA              string    `json:"git_sha"`
	RunMode             string    `json:"run_mode"`
	ExecutionEnabled    bool      `json:"execution_enabled"`
	ExecutionOwner      bool      `json:"execution_owner"`
	FencingToken        int64     `json:"fencing_token,omitempty"`
	SchedulerStatus     string    `json:"scheduler_status"`
	LastAnalysis        time.Time `json:"last_analysis,omitempty"`
	LastReconciliation  time.Time `json:"last_reconciliation,omitempty"`
	LastExchangeSuccess time.Time `json:"last_exchange_success,omitempty"`
	LastSupabaseSync    time.Time `json:"last_supabase_sync,omitempty"`
	OutboxPending       int       `json:"outbox_pending"`
	LastError           string    `json:"last_error,omitempty"`
	UptimeSeconds       int64     `json:"uptime_seconds"`
}
