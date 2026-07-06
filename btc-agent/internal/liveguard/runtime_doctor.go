package liveguard

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent2"
)

const (
	DoctorOK    = "DOCTOR_OK"
	DoctorWarn  = "DOCTOR_WARN"
	DoctorBlock = "DOCTOR_BLOCK"
)

type RuntimeDoctorResult struct {
	GeneratedAt          time.Time             `json:"generated_at"`
	Status               string                `json:"status"`
	AutoLiveEnv          bool                  `json:"auto_live_env"`
	CredentialEnvPresent map[string]bool       `json:"credential_env_present,omitempty"`
	TelegramTokenPresent bool                  `json:"telegram_token_present"`
	TelegramChatPresent  bool                  `json:"telegram_chat_present"`
	OperatorHalted       bool                  `json:"operator_halted"`
	OpenLiveOrders       int                   `json:"open_live_orders"`
	PlanState            agent2.State          `json:"plan_state,omitempty"`
	OKXClientReady       bool                  `json:"okx_client_ready"`
	OKXReadOnlyChecked   bool                  `json:"okx_read_only_checked"`
	ProofStatus          string                `json:"proof_status,omitempty"`
	AccountAuthOK        bool                  `json:"account_auth_ok,omitempty"`
	AccountBalanceOK     bool                  `json:"account_balance_ok,omitempty"`
	PreflightPass        bool                  `json:"preflight_pass,omitempty"`
	DataHealth           DataHealthResult      `json:"data_health,omitempty"`
	DataSanity           DataSanityResult      `json:"data_sanity,omitempty"`
	ReconcileSafety      ReconcileSafetyResult `json:"reconcile_safety,omitempty"`
	RiskGovernor         RiskGovernorResult    `json:"risk_governor,omitempty"`
	Blockers             []string              `json:"blockers,omitempty"`
	Warnings             []string              `json:"warnings,omitempty"`
	Summary              string                `json:"summary"`
}

func (r *RuntimeDoctorResult) RefreshSummary() {
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now()
	}
	r.Blockers = uniqueDoctorStrings(r.Blockers)
	r.Warnings = uniqueDoctorStrings(r.Warnings)
	if len(r.Blockers) > 0 {
		r.Status = DoctorBlock
	} else if len(r.Warnings) > 0 {
		r.Status = DoctorWarn
	} else if r.Status == "" {
		r.Status = DoctorOK
	}
	if r.Summary != "" {
		return
	}
	r.Summary = fmt.Sprintf("%s: blockers=%d warnings=%d open_live_orders=%d okx_ready=%v telegram_ready=%v", r.Status, len(r.Blockers), len(r.Warnings), r.OpenLiveOrders, r.OKXClientReady, r.TelegramTokenPresent && r.TelegramChatPresent)
}

func uniqueDoctorStrings(in []string) []string {
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
