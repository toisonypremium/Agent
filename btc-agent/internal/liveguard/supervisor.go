package liveguard

import (
	"fmt"
	"strings"
	"time"
)

const (
	SupervisorOK     = "SUPERVISOR_OK"
	SupervisorWarn   = "SUPERVISOR_WARN"
	SupervisorHalted = "SUPERVISOR_HALTED"
)

const (
	SupervisorActionManagedCycle  = "managed_cycle"
	SupervisorActionReconcileOnly = "reconcile_only"
	SupervisorActionHeartbeat     = "heartbeat"
	SupervisorActionSkipped       = "skipped"
)

type PortfolioRiskTelemetry struct {
	Known                bool      `json:"known"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
	DrawdownPct          float64   `json:"drawdown_pct,omitempty"`
	DrawdownLockActive   bool      `json:"drawdown_lock_active"`
	DailyRealizedPnL     float64   `json:"daily_realized_pnl,omitempty"`
	DailyLossEquityBasis float64   `json:"daily_loss_equity_basis,omitempty"`
	DailyLossLockActive  bool      `json:"daily_loss_lock_active"`
	Reason               string    `json:"reason,omitempty"`
}

type SupervisorResult struct {
	GeneratedAt       time.Time              `json:"generated_at"`
	Status            string                 `json:"status"`
	Action            string                 `json:"action"`
	ConsecutiveErrors int                    `json:"consecutive_errors"`
	AutoHalted        bool                   `json:"auto_halted,omitempty"`
	Reasons           []string               `json:"reasons,omitempty"`
	Managed           *ManagedCycleResult    `json:"managed,omitempty"`
	Doctor            *RuntimeDoctorResult   `json:"doctor,omitempty"`
	Exits             []ExitDecision         `json:"exits,omitempty"`
	PortfolioRisk     PortfolioRiskTelemetry `json:"portfolio_risk,omitempty"`
	Summary           string                 `json:"summary"`
}

func (r *SupervisorResult) RefreshSummary() {
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now()
	}
	r.Reasons = uniqueSupervisorStrings(r.Reasons)
	if r.Status == "" {
		r.Status = SupervisorOK
	}
	if r.Action == "" {
		r.Action = SupervisorActionHeartbeat
	}
	if r.Managed != nil && supervisorManagedNeedsAttention(r.Managed.Status) {
		r.Reasons = append(r.Reasons, r.Managed.Reasons...)
		for _, decision := range r.Managed.Blocked {
			if decision.Reason != "" {
				r.Reasons = append(r.Reasons, decision.Reason)
			}
			if decision.Error != "" {
				r.Reasons = append(r.Reasons, decision.Error)
			}
		}
		r.Reasons = uniqueSupervisorStrings(r.Reasons)
	}
	if r.AutoHalted {
		r.Status = SupervisorHalted
	} else if r.ConsecutiveErrors > 0 || len(r.Reasons) > 0 || (r.Managed != nil && supervisorManagedNeedsAttention(r.Managed.Status)) {
		r.Status = SupervisorWarn
	}
	if r.Summary != "" {
		return
	}
	parts := []string{fmt.Sprintf("%s: action=%s", r.Status, r.Action)}
	if r.ConsecutiveErrors > 0 {
		parts = append(parts, fmt.Sprintf("errors=%d", r.ConsecutiveErrors))
	}
	if r.Managed != nil {
		parts = append(parts, fmt.Sprintf("managed=%s desired=%d placed=%d canceled=%d replaced=%d blocked=%d", r.Managed.Status, len(r.Managed.Desired), len(r.Managed.Placed), len(r.Managed.Canceled), len(r.Managed.Replaced), len(r.Managed.Blocked)))
	}
	if len(r.Reasons) > 0 {
		parts = append(parts, strings.Join(r.Reasons, "; "))
	}
	r.Summary = strings.Join(parts, " | ")
}

func supervisorManagedNeedsAttention(status string) bool {
	return status == ManagedCycleBlocked || status == ManagedCyclePartial
}

func uniqueSupervisorStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
