package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

func TestControlPlaneSnapshotDoesNotSerializeSecretConfig(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "control-plane.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{}
	cfg.Live.APIKeyEnv = "VERY_SECRET_KEY_ENV"
	cfg.Live.APISecretEnv = "VERY_SECRET_VALUE"
	cfg.HermesOperator.Enabled = true
	cfg.HermesOperator.Mode = "shadow"
	b, err := json.Marshal(buildControlPlaneSnapshot(cfg, db))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, secret := range []string{"VERY_SECRET_KEY_ENV", "VERY_SECRET_VALUE", "api_secret", "api_key_env"} {
		if strings.Contains(text, secret) {
			t.Fatalf("snapshot leaked %q: %s", secret, text)
		}
	}
}

func TestControlPlaneProposalRejectsOversizedFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "proposal-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, 64*1024+1)); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := runControlPlaneValidateProposal(config.Config{}, []string{"btc-agent", "control-plane-validate-proposal", "--proposal-file", f.Name()}); err == nil {
		t.Fatal("oversized proposal accepted")
	}
}

func TestSanitizeControlPlaneValueRemovesCredentialMetadata(t *testing.T) {
	input := map[string]any{"status": "OK", "doctor": map[string]any{"credential_env_present": map[string]any{"OKX_API_SECRET": true}, "summary": "healthy"}, "api_key_env": "KEY"}
	b, err := json.Marshal(sanitizeControlPlaneValue(input))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(b))
	for _, forbidden := range []string{"credential", "okx_api_secret", "api_key_env"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("sanitizer leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "healthy") {
		t.Fatalf("sanitizer removed safe data: %s", text)
	}
}

func TestControlPlaneSubmitProposalIsIdempotentAndShadowOnly(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "proposal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.HermesOperator.DecisionTTLSeconds = 120
	cfg.HermesOperator.MinConfidence = .6
	cfg.HermesOperator.MaxActionsPerCycle = 3
	cfg.HermesOperator.MaxProbeNotionalUSDT = 5
	cfg.HermesOperator.MaxActionNotionalUSDT = 10
	now := time.Now().UTC()
	proposal := map[string]any{"version": 1, "decision_id": "cp-test-1", "generated_at": now, "valid_until": now.Add(60 * time.Second), "market_thesis": "observe", "portfolio_risk_tier": "DEFENSIVE", "actions": []any{}}
	b, _ := json.Marshal(proposal)
	path := filepath.Join(t.TempDir(), "proposal.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"btc-agent", "control-plane-submit-proposal", "--proposal-file", path, "--caller", "nous-hermes"}
	if err := runControlPlaneSubmitProposal(cfg, db, args); err != nil {
		t.Fatal(err)
	}
	if err := runControlPlaneSubmitProposal(cfg, db, args); err != nil {
		t.Fatal(err)
	}
	record, err := db.ControlPlaneProposal("cp-test-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.ExecutionVerdict != "SHADOW_ONLY" {
		t.Fatalf("execution verdict=%s", record.ExecutionVerdict)
	}
	rows, err := db.RecentControlPlaneProposals(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d records", len(rows))
	}
}

func TestControlPlaneProposalRejectsUnknownField(t *testing.T) {
	cfg := config.Config{}
	path := filepath.Join(t.TempDir(), "proposal.json")
	_ = os.WriteFile(path, []byte(`{"version":1,"decision_id":"x","unknown":true}`), 0600)
	if _, _, _, err := readControlPlaneProposal(cfg, []string{"btc-agent", "x", "--proposal-file", path}); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestControlPlaneRequestHaltIsOneWayAndAudited(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "halt.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	args := []string{"btc-agent", "control-plane-request-halt", "--caller", "nous-hermes", "--reason-code", "UNKNOWN_POSITION", "--summary", "synthetic unknown position"}
	if err := runControlPlaneRequestHalt(db, args); err != nil {
		t.Fatal(err)
	}
	halted, err := db.IsHalted()
	if err != nil || !halted {
		t.Fatalf("halted=%v err=%v", halted, err)
	}
	events, err := db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "operator_halt_request" {
		t.Fatalf("events=%+v", events)
	}
}

func TestControlPlaneRequestHaltRejectsUnknownReason(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "halt.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = runControlPlaneRequestHalt(db, []string{"btc-agent", "control-plane-request-halt", "--reason-code", "MODEL_SAID_SO", "--summary", "bad"})
	if err == nil {
		t.Fatal("unknown halt reason accepted")
	}
}
