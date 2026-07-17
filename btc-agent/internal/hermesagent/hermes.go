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

	TriggerSource string `json:"trigger_source,omitempty"`
	TriggerReason string `json:"trigger_reason,omitempty"`
	UserQuestion  string `json:"user_question,omitempty"`

	AuditVerdict          string   `json:"audit_verdict"`
	MarketAuthority       string   `json:"market_authority"`
	CurrentDryRunApproved bool     `json:"current_dry_run_approved"`
	ForcedSimPassed       bool     `json:"forced_sim_passed"`
	AuditReasons          []string `json:"audit_reasons,omitempty"`
	AuditAgeMinutes       int      `json:"audit_age_minutes,omitempty"`

	BTCPhase         string   `json:"btc_phase"`
	BTCPermission    string   `json:"btc_permission"`
	BTCRegime        string   `json:"btc_regime"`
	BTCTrend         float64  `json:"btc_trend_score"`
	BTCMMVerdict     string   `json:"btc_mm_verdict,omitempty"`
	BTCMMConfidence  float64  `json:"btc_mm_confidence,omitempty"`
	BTCMMCoreSignals int      `json:"btc_mm_core_signals,omitempty"`
	BTCMMDataQuality float64  `json:"btc_mm_data_quality,omitempty"`
	PlanState        string   `json:"plan_state"`
	DoctorStatus     string   `json:"doctor_status"`
	DoctorBlockers   []string `json:"doctor_blockers,omitempty"`

	Assets      []HermesAsset `json:"assets,omitempty"`
	ExitEnabled bool          `json:"exit_enabled"`
	Exits       []HermesExit  `json:"exits,omitempty"`

	Positions []HermesPosition `json:"positions,omitempty"`

	ResearchSummary string `json:"research_summary,omitempty"`

	SchedulerRunning bool   `json:"scheduler_running"`
	OperatorHalted   bool   `json:"operator_halted"`
	LastSupervisorAt string `json:"last_supervisor_at,omitempty"`
}

type HermesAsset struct {
	Symbol         string   `json:"symbol"`
	State          string   `json:"state"`
	Readiness      float64  `json:"readiness_pct"`
	RR             float64  `json:"reward_risk"`
	OpenOrders     int      `json:"open_orders"`
	Why            string   `json:"why,omitempty"`
	EntryZoneLow   float64  `json:"entry_zone_low,omitempty"`
	EntryZoneHigh  float64  `json:"entry_zone_high,omitempty"`
	Invalidation   float64  `json:"invalidation,omitempty"`
	Target         float64  `json:"target,omitempty"`
	MMCase         string   `json:"mm_case,omitempty"`
	MMScore        float64  `json:"mm_score,omitempty"`
	MMMissing      []string `json:"mm_missing,omitempty"`
	FlowBias       string   `json:"flow_bias,omitempty"`
	FlowScore      float64  `json:"flow_score,omitempty"`
	LiquidityGrade string   `json:"liquidity_grade,omitempty"`
	LiquidityScore float64  `json:"liquidity_score,omitempty"`
	LiquidityPass  bool     `json:"liquidity_pass,omitempty"`
	RotationRank   int      `json:"rotation_rank,omitempty"`
	RotationScore  float64  `json:"rotation_score,omitempty"`
	NextTrigger    string   `json:"next_trigger,omitempty"`
	ProbeEligible  bool     `json:"probe_eligible"`
	ProbePolicy    string   `json:"probe_policy,omitempty"`
}

type HermesExit struct {
	Symbol string  `json:"symbol"`
	Action string  `json:"action"`
	PnLPct float64 `json:"pnl_pct"`
	Reason string  `json:"reason,omitempty"`
}

type HermesPosition struct {
	Symbol        string  `json:"symbol"`
	Quantity      float64 `json:"quantity"`
	AvgEntryPrice float64 `json:"avg_entry_price"`
	OpenedAt      int64   `json:"opened_at,omitempty"`
}

type HermesReport struct {
	GeneratedAt  time.Time `json:"generated_at"`
	GateSummary  string    `json:"gate_summary"`
	AssetSummary string    `json:"asset_summary"`
	ExitSummary  string    `json:"exit_summary"`
	ActionLine   string    `json:"action_line"`
	Anomalies    []string  `json:"anomalies,omitempty"`
	TelegramText string    `json:"telegram_text"`
	WorthyAlert  bool      `json:"worthy_alert"`
}

type HermesState struct {
	LastSentFingerprint string    `json:"last_sent_fingerprint,omitempty"`
	LastAuditVerdict    string    `json:"last_audit_verdict,omitempty"`
	LastDoctorStatus    string    `json:"last_doctor_status,omitempty"`
	LastExitFingerprint string    `json:"last_exit_fingerprint,omitempty"`
	LastSentAt          time.Time `json:"last_sent_at,omitempty"`
}

type HermesTrigger struct {
	Source      string `json:"source"`
	Reason      string `json:"reason"`
	UserText    string `json:"user_text,omitempty"`
	ForceReply  bool   `json:"force_reply"`
	AllowNotify bool   `json:"allow_notify"`
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
	if report.ActionLine == "" {
		report.ActionLine = "READ_ONLY — no order placed."
	}
	if report.TelegramText == "" {
		return fallback(snap), nil
	}
	if forbiddenExecution(report.TelegramText) || forbiddenExecution(report.ActionLine) {
		return fallback(snap), nil
	}
	if strings.TrimSpace(report.ActionLine) != "READ_ONLY — no order placed." {
		report.ActionLine = "READ_ONLY — no order placed."
	}
	report.WorthyAlert = hasExitSignal(snap) || snap.AuditVerdict == "APPROVED_DRY_RUN" || snap.AuditVerdict == "APPROVED_REAL_ORDER"
	return report, nil
}

func forbiddenExecution(s string) bool {
	lower := strings.ToLower(s)
	for _, f := range []string{"đặt lệnh", "place order", "order placed", "buy order", "sell order", "cancel order", "execute sell", "thực thi"} {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
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
	return fmt.Sprintf(`Return exactly one valid JSON object. No markdown.
You are Hermes, read-only AI manager for btc-agent.
You must answer in Vietnamese.
You never place, cancel, or modify orders.
If UserQuestion is non-empty, answer that question directly, then give short bot summary.
Always include action_line exactly as: "READ_ONLY — no order placed."
Max telegram_text 1400 chars.

JSON schema:
{
  "gate_summary": "1-2 câu tóm tắt trạng thái gate",
  "asset_summary": "1-2 câu tóm tắt asset",
  "exit_summary": "1 câu tóm tắt exit signal hoặc NONE",
  "action_line": "READ_ONLY — no order placed.",
  "anomalies": ["..."],
  "telegram_text": "Telegram-ready Vietnamese text",
  "worthy_alert": false
}

Rules:
- worthy_alert = true only if exit signal non-HOLD OR audit = APPROVED_DRY_RUN/APPROVED_REAL_ORDER OR trigger asks for reply
- Never suggest placing, canceling, or modifying any order
- Never print secrets or credentials
- Keep text concise and operational

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
	telegram := fmt.Sprintf("HERMES BOT MANAGER\n\n📊 Gate: %s\nAudit: %s\nBTC: %s | Permission: %s\n\n📈 Assets: %s\n\n📉 Exits: %s\n\n✅ READ_ONLY — no order placed.",
		gate, snap.AuditVerdict, snap.BTCPhase, snap.BTCPermission, assetStr, exits)
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
