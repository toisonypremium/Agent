package hermesagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JSONCaller matches internal/llm.Client interface.
type JSONCaller interface {
	ChatJSON(ctx context.Context, prompt string, out any) error
}

// HermesSnapshot is the input context assembled from all report files.
type HermesSnapshot struct {
	GeneratedAt time.Time `json:"generated_at"`

	// Gate state (from live_auto_audit_latest.json)
	AuditVerdict         string   `json:"audit_verdict"`
	MarketAuthority      string   `json:"market_authority"`
	CurrentDryRunApproved bool    `json:"current_dry_run_approved"`
	ForcedSimPassed      bool     `json:"forced_sim_passed"`
	AuditReasons         []string `json:"audit_reasons,omitempty"`
	AuditAge             string   `json:"audit_age_minutes"`

	// BTC + plan (from audit analysis/plan)
	BTCPhase       string  `json:"btc_phase"`
	BTCPermission  string  `json:"btc_permission"`
	BTCRegime      string  `json:"btc_regime"`
	BTCTrend       float64 `json:"btc_trend_score"`
	PlanState      string  `json:"plan_state"`
	DoctorStatus   string  `json:"doctor_status"`
	DoctorBlockers []string `json:"doctor_blockers,omitempty"`

	// Assets (from supervisor/scenario)
	Assets []HermesAsset `json:"assets,omitempty"`

	// Exit state (from supervisor exits)
	ExitEnabled bool          `json:"exit_enabled"`
	Exits       []HermesExit  `json:"exits,omitempty"`

	// Positions
	Positions []HermesPosition `json:"positions,omitempty"`

	// Research brief summary
	ResearchSummary string `json:"research_summary,omitempty"`

	// Scheduler / ops
	SchedulerRunning bool   `json:"scheduler_running"`
	OperatorHalted   bool   `json:"operator_halted"`
	LastSupervisorAt string `json:"last_supervisor_at,omitempty"`
}

type HermesAsset struct {
	Symbol    string  `json:"symbol"`
	State     string  `json:"state"`
	Readiness float64 `json:"readiness_pct"`
	RR        float64 `json:"reward_risk"`
	OpenOrders int    `json:"open_orders"`
	Why       string  `json:"why,omitempty"`
}

type HermesExit struct {
	Symbol   string  `json:"symbol"`
	Action   string  `json:"action"`
	PnLPct   float64 `json:"pnl_pct"`
	Reason   string  `json:"reason,omitempty"`
}

type HermesPosition struct {
	Symbol        string  `json:"symbol"`
	Quantity      float64 `json:"quantity"`
	AvgEntryPrice float64 `json:"avg_entry_price"`
	OpenedAt      int64   `json:"opened_at,omitempty"`
}

// HermesReport is the LLM output.
type HermesReport struct {
	GeneratedAt   time.Time `json:"generated_at"`
	GateSummary   string    `json:"gate_summary"`
	AssetSummary  string    `json:"asset_summary"`
	ExitSummary   string    `json:"exit_summary"`
	ActionLine    string    `json:"action_line"`
	Anomalies     []string  `json:"anomalies,omitempty"`
	TelegramText  string    `json:"telegram_text"`
	WorthyAlert   bool      `json:"worthy_alert"`
}

// Generate calls the LLM to produce a HermesReport.
// If caller is nil, returns a deterministic fallback.
func Generate(ctx context.Context, caller JSONCaller, snap HermesSnapshot) (HermesReport, error) {
	snap.GeneratedAt = time.Now().UTC()
	if caller == nil {
		return fallback(snap), nil
	}
	var report HermesReport
	if err := caller.ChatJSON(ctx, buildPrompt(snap), &report); err != nil {
		return fallback(snap), err
	}
	report.GeneratedAt = snap.GeneratedAt
	// Safety: Hermes must never claim order was placed
	forbidden := []string{"đặt lệnh", "place order", "order placed", "buy order", "sell order", "cancel order"}
	for _, f := range forbidden {
		if strings.Contains(strings.ToLower(report.TelegramText), f) ||
			strings.Contains(strings.ToLower(report.ActionLine), f) {
			return fallback(snap), nil
		}
	}
	if report.ActionLine == "" {
		report.ActionLine = "READ_ONLY — no order placed."
	}
	if report.TelegramText == "" {
		return fallback(snap), nil
	}
	// WorthyAlert: send Telegram only if there is an exit signal or audit change
	report.WorthyAlert = hasExitSignal(snap) || snap.AuditVerdict == "APPROVED_DRY_RUN" || snap.AuditVerdict == "APPROVED_REAL_ORDER"
	return report, nil
}

func hasExitSignal(snap HermesSnapshot) bool {
	for _, e := range snap.Exits {
		if e.Action != "HOLD" && e.Action != "" {
			return true
		}
	}
	return false
}

func buildPrompt(snap HermesSnapshot) string {
	payload, _ := json.MarshalIndent(snap, "", "  ")
	return fmt.Sprintf(`Return exactly one valid JSON object matching this schema. No markdown, no explanation outside JSON.
You are Hermes, read-only AI manager for a BTC-gated accumulation bot.
The bot is a deterministic Go system. You ONLY read and report. You never place, cancel, or suggest orders.
Write in Vietnamese. Max telegram_text 1400 chars.

JSON schema (all fields required):
{
  "gate_summary": "1-2 câu tóm tắt trạng thái gate: audit verdict, BTC phase, blocker chính",
  "asset_summary": "1-2 câu tóm tắt từng asset: state, readiness, tại sao chưa vào lệnh",
  "exit_summary": "1 câu: exit signals nếu có, NONE nếu không",
  "action_line": "READ_ONLY — no order placed.",
  "anomalies": ["danh sách bất thường đáng chú ý, hoặc []"],
  "telegram_text": "Hermes report đầy đủ cho Telegram, Vietnamese, max 1400 chars",
  "worthy_alert": false
}

Rules:
- action_line MUST always be exactly: "READ_ONLY — no order placed."
- worthy_alert = true only if exit signal non-HOLD OR audit = APPROVED_DRY_RUN/APPROVED_REAL_ORDER
- Never suggest placing, canceling, or modifying any order
- Never print API keys, secrets, or credentials

Snapshot:
%s`, string(payload))
}

func fallback(snap HermesSnapshot) HermesReport {
	gate := fmt.Sprintf("Gate: %s | BTC: %s | Doctor: %s", snap.AuditVerdict, snap.BTCPhase, snap.DoctorStatus)
	exits := "NONE"
	if hasExitSignal(snap) {
		parts := []string{}
		for _, e := range snap.Exits {
			if e.Action != "HOLD" {
				parts = append(parts, fmt.Sprintf("%s→%s(%.1f%%)", e.Symbol, e.Action, e.PnLPct*100))
			}
		}
		exits = strings.Join(parts, ", ")
	}
	assets := []string{}
	for _, a := range snap.Assets {
		assets = append(assets, fmt.Sprintf("%s=%s(%.0f%%)", a.Symbol, a.State, a.Readiness))
	}
	assetStr := strings.Join(assets, " ")
	telegram := fmt.Sprintf("HERMES BOT MANAGER\n\n📊 Gate: %s\nAudit: %s\n\n📈 Assets: %s\n\n📉 Exits: %s\n\n✅ READ_ONLY — no order placed.",
		gate, snap.AuditVerdict, assetStr, exits)
	if len(telegram) > 1400 {
		telegram = telegram[:1397] + "..."
	}
	return HermesReport{
		GeneratedAt:  snap.GeneratedAt,
		GateSummary:  gate,
		AssetSummary: assetStr,
		ExitSummary:  exits,
		ActionLine:   "READ_ONLY — no order placed.",
		TelegramText: telegram,
		WorthyAlert:  hasExitSignal(snap),
	}
}
