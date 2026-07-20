package circuitresearch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

func fixture(t *testing.T) (InputSnapshot, time.Time) {
	t.Helper()
	now := time.Date(2026, 7, 20, 2, 0, 0, 0, time.UTC)
	a := agent1.MarketAnalysis{Timestamp: now, BTCPrice: 100, MarketRegime: "RANGE", RiskLevel: agent1.Medium, ActionPermission: agent1.Watch}
	p := agent2.Plan{Timestamp: now, State: agent2.StateWatch, ActionPermission: agent1.Watch}
	in, err := BuildInput(a, p, Producer{Name: "btc-agent", Version: "test", Commit: "abc"}, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return in, now
}
func TestBuildInputDeterministicAndResearchOnly(t *testing.T) {
	a, _ := fixture(t)
	b, _ := fixture(t)
	if a.SnapshotID != b.SnapshotID || a.InputSHA256 != b.InputSHA256 {
		t.Fatal("unstable hash")
	}
	if !a.Policy.ResearchOnly || a.Policy.ExecutionAuthority || a.Policy.ShortAllowed || a.Policy.LeverageAllowed {
		t.Fatal("unsafe policy")
	}
	if err := ValidateInputHash(a); err != nil {
		t.Fatal(err)
	}
}
func TestInputTamperRejected(t *testing.T) {
	in, _ := fixture(t)
	in.Market.BTCPrice++
	if ValidateInputHash(in) == nil {
		t.Fatal("tamper accepted")
	}
}
func validEvidence(t *testing.T) (Evidence, ValidationPolicy) {
	in, now := fixture(t)
	e := Evidence{SchemaVersion: EvidenceSchemaVersion, RunID: "run-1", InputSnapshotID: in.SnapshotID, InputSHA256: in.InputSHA256, GeneratedAt: now, ValidUntil: now.Add(5 * time.Minute), ProducerCommit: "circuit-sha", Status: "COMPLETE", ResearchAction: ActionWatch, Confidence: .5, Evidence: []EvidenceItem{{Code: "REGIME", Metric: "regime", Value: "range", Source: "btc-agent", SourceTimestamp: now}}, Authority: "RESEARCH_ONLY", ExecutionIntent: nil}
	e, err := SignEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	return e, ValidationPolicy{Now: now, ExpectedInput: in, AllowedProducerCommit: "circuit-sha", MaxTTL: 15 * time.Minute}
}
func TestEvidenceValidation(t *testing.T) {
	e, p := validEvidence(t)
	raw, _ := json.Marshal(e)
	r := DecodeAndValidateEvidence(raw, p)
	if !r.Valid {
		t.Fatal(r.ReasonCodes)
	}
}
func TestForbiddenAuthorityRejected(t *testing.T) {
	e, p := validEvidence(t)
	e.ExecutionIntent = "PROBE_LIMIT"
	e, _ = SignEvidence(e)
	raw, _ := json.Marshal(e)
	r := DecodeAndValidateEvidence(raw, p)
	if r.Valid || !contains(r.ReasonCodes, "CIRCUIT_AUTHORITY_FORBIDDEN") {
		t.Fatal(r.ReasonCodes)
	}
}
func TestUnknownFieldRejected(t *testing.T) {
	e, p := validEvidence(t)
	raw, _ := json.Marshal(e)
	raw = append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...)
	r := DecodeAndValidateEvidence(raw, p)
	if r.Valid || !contains(r.ReasonCodes, "CIRCUIT_OUTPUT_INVALID_JSON") {
		t.Fatal(r.ReasonCodes)
	}
}
func TestInputBindingRejected(t *testing.T) {
	e, p := validEvidence(t)
	e.InputSnapshotID = "wrong"
	e, _ = SignEvidence(e)
	raw, _ := json.Marshal(e)
	r := DecodeAndValidateEvidence(raw, p)
	if r.Valid || !contains(r.ReasonCodes, "CIRCUIT_INPUT_BINDING_MISMATCH") {
		t.Fatal(r.ReasonCodes)
	}
}
func FuzzCircuitEvidence(f *testing.F) {
	e, p := validEvidence(&testing.T{})
	seed, _ := json.Marshal(e)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, raw []byte) { _ = DecodeAndValidateEvidence(raw, p) })
}
func contains(s []string, w string) bool {
	for _, v := range s {
		if v == w {
			return true
		}
	}
	return false
}

func TestDeterministicAdapterCrossLanguage(t *testing.T) {
	python := os.Getenv("CIRCUIT_TEST_PYTHON")
	if python == "" {
		var err error
		python, err = exec.LookPath("python3")
		if err != nil {
			t.Skip("python3 unavailable")
		}
	}
	adapter := os.Getenv("CIRCUIT_ADAPTER_PATH")
	if adapter == "" {
		adapter = "/data/data/com.termux/files/home/circuit-framework-review/adapter/run_deterministic.py"
	}
	if _, err := os.Stat(adapter); err != nil {
		t.Skipf("Circuit adapter unavailable: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	analysis := agent1.MarketAnalysis{Timestamp: now, BTCPrice: 100, MarketRegime: "RANGE", RiskLevel: agent1.Medium, ActionPermission: agent1.Watch}
	plan := agent2.Plan{Timestamp: now, State: agent2.StateWatch, ActionPermission: agent1.Watch}
	input, err := BuildInput(analysis, plan, Producer{Name: "btc-agent", Version: "test", Commit: "abc"}, now, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	outputPath := filepath.Join(dir, "evidence.json")
	raw, _ := json.Marshal(input)
	if err := os.WriteFile(inputPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, adapter, "--input", inputPath, "--output", outputPath, "--producer-commit", "circuit-sha")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("adapter: %v: %s", err, output)
	}
	evidenceRaw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	result := DecodeAndValidateEvidence(evidenceRaw, ValidationPolicy{Now: time.Now().UTC(), ExpectedInput: input, AllowedProducerCommit: "circuit-sha", MaxTTL: 15 * time.Minute})
	if !result.Valid {
		t.Log(string(evidenceRaw))
		t.Fatal(result.ReasonCodes)
	}
	if result.Evidence.ResearchAction != ActionNoTrade || result.Evidence.Authority != "RESEARCH_ONLY" {
		t.Fatalf("unsafe evidence: %+v", result.Evidence)
	}
}
