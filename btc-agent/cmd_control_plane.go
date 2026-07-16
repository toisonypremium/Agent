package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/storage"
)

const controlPlaneSchemaVersion = 1

type ControlPlaneSnapshot struct {
	SchemaVersion int       `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Operator      struct {
		Halted             bool   `json:"halted"`
		HermesEnabled      bool   `json:"hermes_enabled"`
		HermesMode         string `json:"hermes_mode"`
		ExecutionAuthority bool   `json:"execution_authority"`
	} `json:"operator"`
	Market         any                   `json:"market,omitempty"`
	Plan           any                   `json:"plan,omitempty"`
	Positions      any                   `json:"positions,omitempty"`
	OpenOrders     any                   `json:"open_orders,omitempty"`
	LatestDecision *hermesShadowDecision `json:"latest_decision,omitempty"`
	Reports        map[string]any        `json:"reports,omitempty"`
	Warnings       []string              `json:"warnings,omitempty"`
}

// buildControlPlaneSnapshot exposes only strategy/runtime data. Config secrets,
// environment variables and raw exchange credentials are never serialized.
func buildControlPlaneSnapshot(cfg config.Config, db *storage.DB) ControlPlaneSnapshot {
	out := ControlPlaneSnapshot{SchemaVersion: controlPlaneSchemaVersion, GeneratedAt: time.Now().UTC(), Reports: map[string]any{}}
	out.Operator.HermesEnabled = cfg.HermesOperator.Enabled
	out.Operator.HermesMode = cfg.HermesOperator.NormalizedMode()
	out.Operator.ExecutionAuthority = cfg.HermesOperator.CanExecute()
	if halted, err := db.IsHalted(); err == nil {
		out.Operator.Halted = halted
	} else {
		out.Warnings = append(out.Warnings, "operator status unavailable")
	}
	if analysis, err := db.LatestAnalysis(); err == nil {
		out.Market = analysis
	} else {
		out.Warnings = append(out.Warnings, "analysis unavailable")
	}
	if plan, err := db.LatestPlan(); err == nil {
		out.Plan = plan
	} else {
		out.Warnings = append(out.Warnings, "plan unavailable")
	}
	if positions, err := db.LivePositions(); err == nil {
		out.Positions = positions
	} else {
		out.Warnings = append(out.Warnings, "positions unavailable")
	}
	if orders, err := db.OpenLiveOrdersDetailed(); err == nil {
		out.OpenOrders = orders
	} else {
		out.Warnings = append(out.Warnings, "open orders unavailable")
	}
	if b, err := os.ReadFile(filepath.Join("reports", "hermes_shadow_decision_latest.json")); err == nil {
		var d hermesShadowDecision
		if json.Unmarshal(b, &d) == nil {
			out.LatestDecision = &d
		}
	}
	for _, name := range []string{"live_auto_audit_latest.json", "live_supervisor_latest.json", "auto_live_management_latest.json"} {
		if b, err := os.ReadFile(filepath.Join("reports", name)); err == nil && json.Valid(b) {
			var report any
			if json.Unmarshal(b, &report) == nil {
				out.Reports[name] = sanitizeControlPlaneValue(report)
			}
		}
	}
	return out
}

func runControlPlaneSnapshot(cfg config.Config, db *storage.DB) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(buildControlPlaneSnapshot(cfg, db))
}

type controlPlaneProposalReceipt struct {
	DecisionID       string    `json:"decision_id"`
	Caller           string    `json:"caller"`
	ReceivedAt       time.Time `json:"received_at"`
	PayloadSHA256    string    `json:"payload_sha256"`
	SchemaVerdict    string    `json:"schema_verdict"`
	PolicyVerdict    string    `json:"policy_verdict"`
	ExecutionVerdict string    `json:"execution_verdict"`
	Reasons          []string  `json:"reasons"`
	Duplicate        bool      `json:"duplicate"`
}

func readControlPlaneProposal(cfg config.Config, args []string) (hermesoperator.Decision, []byte, hermesoperator.ValidationResult, error) {
	path := argValue(args, "--proposal-file")
	if path == "" {
		return hermesoperator.Decision{}, nil, hermesoperator.ValidationResult{}, fmt.Errorf("--proposal-file required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return hermesoperator.Decision{}, nil, hermesoperator.ValidationResult{}, err
	}
	if info.Size() > 64*1024 {
		return hermesoperator.Decision{}, nil, hermesoperator.ValidationResult{}, fmt.Errorf("proposal exceeds 64 KiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return hermesoperator.Decision{}, nil, hermesoperator.ValidationResult{}, err
	}
	var decision hermesoperator.Decision
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decision); err != nil {
		return decision, b, hermesoperator.ValidationResult{}, fmt.Errorf("decode proposal: %w", err)
	}
	if dec.More() {
		return decision, b, hermesoperator.ValidationResult{}, fmt.Errorf("decode proposal: trailing JSON")
	}
	allowed := map[string]bool{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		allowed[symbol] = true
	}
	result := hermesoperator.Validate(decision, hermesoperator.ValidationPolicy{Now: time.Now().UTC(), MaxDecisionTTL: time.Duration(cfg.HermesOperator.DecisionTTLSeconds) * time.Second, MinConfidence: cfg.HermesOperator.MinConfidence, MaxActions: cfg.HermesOperator.MaxActionsPerCycle, MaxProbeNotionalUSDT: config.EffectiveHermesProbeNotional(cfg), MaxActionNotionalUSDT: config.EffectiveHermesActionNotional(cfg), AllowedSymbols: allowed})
	return decision, b, result, nil
}

func runControlPlaneValidateProposal(cfg config.Config, args []string) error {
	_, _, result, err := readControlPlaneProposal(cfg, args)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
	if len(result.Reasons) > 0 {
		return fmt.Errorf("proposal rejected")
	}
	return nil
}

func runControlPlaneSubmitProposal(cfg config.Config, db *storage.DB, args []string) error {
	caller := argValue(args, "--caller")
	if caller == "" {
		caller = "nous-hermes"
	}
	if caller != "nous-hermes" {
		return fmt.Errorf("caller not allowed")
	}
	decision, b, result, err := readControlPlaneProposal(cfg, args)
	if err != nil {
		return err
	}
	reasons := append([]string(nil), result.Reasons...)
	policy := "ACCEPTED"
	if len(reasons) > 0 {
		policy = "REJECTED"
	}
	h := sha256.Sum256(b)
	reasonsBytes, _ := json.Marshal(reasons)
	now := time.Now().UTC()
	record := storage.ControlPlaneProposal{DecisionID: decision.DecisionID, Caller: caller, ReceivedAt: now, PayloadSHA256: hex.EncodeToString(h[:]), PayloadJSON: string(b), SchemaVerdict: "VALID", PolicyVerdict: policy, ExecutionVerdict: "SHADOW_ONLY", ReasonsJSON: string(reasonsBytes)}
	inserted, err := db.SaveControlPlaneProposal(record)
	if err != nil {
		return err
	}
	if !inserted {
		existing, e := db.ControlPlaneProposal(decision.DecisionID)
		if e != nil {
			return e
		}
		if existing.PayloadSHA256 != record.PayloadSHA256 {
			return fmt.Errorf("decision_id replay with different payload")
		}
		record = existing
	}
	receipt := controlPlaneProposalReceipt{DecisionID: record.DecisionID, Caller: record.Caller, ReceivedAt: record.ReceivedAt, PayloadSHA256: record.PayloadSHA256, SchemaVerdict: record.SchemaVerdict, PolicyVerdict: record.PolicyVerdict, ExecutionVerdict: record.ExecutionVerdict, Reasons: reasons, Duplicate: !inserted}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(receipt)
}

func runControlPlaneProposalResult(db *storage.DB, args []string) error {
	id := argValue(args, "--decision-id")
	if id == "" {
		return fmt.Errorf("--decision-id required")
	}
	p, err := db.ControlPlaneProposal(id)
	if err != nil {
		return err
	}
	p.PayloadJSON = ""
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
func runControlPlaneRecentProposals(db *storage.DB) error {
	rows, err := db.RecentControlPlaneProposals(20)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func sanitizeControlPlaneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "credential") || strings.Contains(lower, "api_key") || strings.Contains(lower, "api_secret") || strings.Contains(lower, "passphrase") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "environment") || strings.HasSuffix(lower, "_env") {
				continue
			}
			out[key] = sanitizeControlPlaneValue(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = sanitizeControlPlaneValue(child)
		}
		return out
	default:
		return value
	}
}

type controlPlaneHaltReceipt struct {
	Status     string    `json:"status"`
	ReasonCode string    `json:"reason_code"`
	Summary    string    `json:"summary"`
	Caller     string    `json:"caller"`
	HaltedAt   time.Time `json:"halted_at"`
	Duplicate  bool      `json:"duplicate"`
}

func runControlPlaneRequestHalt(db *storage.DB, args []string) error {
	caller := argValue(args, "--caller")
	if caller == "" {
		caller = "nous-hermes"
	}
	if caller != "nous-hermes" {
		return fmt.Errorf("caller not allowed")
	}
	reason := strings.ToUpper(strings.TrimSpace(argValue(args, "--reason-code")))
	allowed := map[string]bool{"RECONCILE_MISMATCH": true, "UNKNOWN_ORDER": true, "UNKNOWN_POSITION": true, "EXPOSURE_BREACH": true, "EXECUTION_UNCERTAINTY": true}
	if !allowed[reason] {
		return fmt.Errorf("invalid halt reason code")
	}
	summary := strings.TrimSpace(argValue(args, "--summary"))
	if summary == "" || len(summary) > 500 {
		return fmt.Errorf("halt summary required and must be <=500 characters")
	}
	wasHalted, err := db.IsHalted()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"caller": caller, "reason_code": reason, "summary": summary, "halted_at": now})
	if err := db.SaveRuntimeEvent(storage.RuntimeEvent{Timestamp: now, Source: "nous-hermes", Type: "operator_halt_request", Severity: "critical", Fingerprint: "halt:" + reason + ":" + now.Format("200601021504"), PayloadJSON: string(payload)}); err != nil {
		return err
	}
	if err := db.SetHaltStatus(true); err != nil {
		return err
	}
	out := controlPlaneHaltReceipt{Status: "HALTED", ReasonCode: reason, Summary: summary, Caller: caller, HaltedAt: now, Duplicate: wasHalted}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
