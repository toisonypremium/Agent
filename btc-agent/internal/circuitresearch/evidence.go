package circuitresearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

const EvidenceSchemaVersion = "circuit-research-evidence-v1"

type ResearchAction string

const (
	ActionWatch       ResearchAction = "WATCH"
	ActionNoTrade     ResearchAction = "NO_TRADE"
	ActionInvestigate ResearchAction = "INVESTIGATE"
)

type EvidenceItem struct {
	Code            string    `json:"code"`
	Metric          string    `json:"metric"`
	Value           any       `json:"value"`
	Source          string    `json:"source"`
	SourceTimestamp time.Time `json:"source_timestamp"`
}
type Evidence struct {
	SchemaVersion   string         `json:"schema_version"`
	RunID           string         `json:"run_id"`
	InputSnapshotID string         `json:"input_snapshot_id"`
	InputSHA256     string         `json:"input_sha256"`
	GeneratedAt     time.Time      `json:"generated_at"`
	ValidUntil      time.Time      `json:"valid_until"`
	ProducerCommit  string         `json:"producer_commit"`
	Status          string         `json:"status"`
	ResearchAction  ResearchAction `json:"research_action"`
	Confidence      float64        `json:"confidence"`
	Regime          map[string]any `json:"regime,omitempty"`
	Evidence        []EvidenceItem `json:"evidence"`
	Limitations     []string       `json:"limitations,omitempty"`
	Authority       string         `json:"authority"`
	ExecutionIntent any            `json:"execution_intent"`
	OutputSHA256    string         `json:"output_sha256"`
}
type ValidationPolicy struct {
	Now                   time.Time
	ExpectedInput         InputSnapshot
	AllowedProducerCommit string
	MaxTTL                time.Duration
}
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
	Evidence    Evidence `json:"evidence"`
}

func DecodeAndValidateEvidence(raw []byte, p ValidationPolicy) ValidationResult {
	reasons := []string{}
	if len(raw) > MaxPayloadBytes {
		return ValidationResult{ReasonCodes: []string{"CIRCUIT_OUTPUT_TOO_LARGE"}}
	}
	var e Evidence
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return ValidationResult{ReasonCodes: []string{"CIRCUIT_OUTPUT_INVALID_JSON"}}
	}
	if err := ensureEOF(dec); err != nil {
		return ValidationResult{ReasonCodes: []string{"CIRCUIT_OUTPUT_TRAILING_JSON"}}
	}
	now := p.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if p.MaxTTL <= 0 {
		p.MaxTTL = 15 * time.Minute
	}
	if e.SchemaVersion != EvidenceSchemaVersion {
		reasons = append(reasons, "CIRCUIT_OUTPUT_SCHEMA_UNSUPPORTED")
	}
	if strings.TrimSpace(e.RunID) == "" {
		reasons = append(reasons, "CIRCUIT_RUN_ID_MISSING")
	}
	if e.InputSnapshotID != p.ExpectedInput.SnapshotID || e.InputSHA256 != p.ExpectedInput.InputSHA256 {
		reasons = append(reasons, "CIRCUIT_INPUT_BINDING_MISMATCH")
	}
	if e.ProducerCommit != p.AllowedProducerCommit {
		reasons = append(reasons, "CIRCUIT_PRODUCER_MISMATCH")
	}
	if e.GeneratedAt.IsZero() || e.ValidUntil.IsZero() || !now.Before(e.ValidUntil) || e.GeneratedAt.After(now.Add(2*time.Minute)) || e.ValidUntil.Sub(e.GeneratedAt) > p.MaxTTL {
		reasons = append(reasons, "CIRCUIT_OUTPUT_TIMESTAMP_INVALID")
	}
	if e.Status != "COMPLETE" && e.Status != "PARTIAL" && e.Status != "FAILED" {
		reasons = append(reasons, "CIRCUIT_STATUS_INVALID")
	}
	if e.ResearchAction != ActionWatch && e.ResearchAction != ActionNoTrade && e.ResearchAction != ActionInvestigate {
		reasons = append(reasons, "CIRCUIT_ACTION_FORBIDDEN")
	}
	if math.IsNaN(e.Confidence) || math.IsInf(e.Confidence, 0) || e.Confidence < 0 || e.Confidence > 1 {
		reasons = append(reasons, "CIRCUIT_CONFIDENCE_INVALID")
	}
	if e.Authority != "RESEARCH_ONLY" || e.ExecutionIntent != nil {
		reasons = append(reasons, "CIRCUIT_AUTHORITY_FORBIDDEN")
	}
	for _, item := range e.Evidence {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Source) == "" || item.SourceTimestamp.IsZero() {
			reasons = append(reasons, "CIRCUIT_EVIDENCE_PROVENANCE_MISSING")
			break
		}
		if item.SourceTimestamp.After(now.Add(2 * time.Minute)) {
			reasons = append(reasons, "CIRCUIT_EVIDENCE_TIMESTAMP_INVALID")
			break
		}
	}
	canonical, err := canonicalRawEvidence(raw)
	if err != nil {
		reasons = append(reasons, "CIRCUIT_OUTPUT_HASH_INVALID")
	} else {
		h := sha256.Sum256(canonical)
		if e.OutputSHA256 != hex.EncodeToString(h[:]) {
			reasons = append(reasons, "CIRCUIT_OUTPUT_HASH_MISMATCH")
		}
	}
	return ValidationResult{Valid: len(reasons) == 0, ReasonCodes: dedupe(reasons), Evidence: e}
}
func SignEvidence(e Evidence) (Evidence, error) {
	raw, err := canonicalEvidence(e)
	if err != nil {
		return Evidence{}, err
	}
	h := sha256.Sum256(raw)
	e.OutputSHA256 = hex.EncodeToString(h[:])
	return e, nil
}
func canonicalEvidence(e Evidence) ([]byte, error) {
	e.OutputSHA256 = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	var canonical any
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func canonicalRawEvidence(raw []byte) ([]byte, error) {
	var canonical map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	canonical["output_sha256"] = ""
	return json.Marshal(canonical)
}
func ensureEOF(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	return fmt.Errorf("trailing JSON")
}
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
