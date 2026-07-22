package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liveguard"
)

func TestNoActionHermesCycleIsSuccessfulDryRun(t *testing.T) {
	got := noActionHermesCycle(agent2.Plan{State: agent2.StateScout}, true, "valid hold")
	if got.Status != liveguard.ManagedCycleDryRun || got.Summary == "" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestInvalidHermesCycleRemainsBlocked(t *testing.T) {
	got := blockedHermesCycle(agent2.Plan{}, false, "invalid")
	if got.Status != liveguard.ManagedCycleBlocked {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestHermesActionsIncreaseExposureOnlyForBuyIntents(t *testing.T) {
	if hermesActionsIncreaseExposure([]hermesoperator.Action{{Intent: hermesoperator.IntentCancel}}) {
		t.Fatal("risk-reducing action classified as exposure increase")
	}
	if !hermesActionsIncreaseExposure([]hermesoperator.Action{{Intent: hermesoperator.IntentProbeLimit}}) {
		t.Fatal("probe exposure increase not detected")
	}
}

func TestLoadHermesCanaryReadinessRejectsMalformedAndLoadsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readiness.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadHermesCanaryReadiness(path); err == nil {
		t.Fatal("malformed readiness must fail")
	}
	payload := `{"generated_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","verdict":"READY"}`
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadHermesCanaryReadiness(path); err != nil || got.Verdict != liveguard.CanaryReady {
		t.Fatalf("valid readiness load failed: %+v err=%v", got, err)
	}
}

func TestLatestLifecycleQualificationSelectsNewestWithHash(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, generated time.Time) {
		t.Helper()
		payload := fmt.Sprintf(`{"generated_at":%q,"qualification":"HERMES_SYNTHETIC_LIFECYCLE","result":"PASS","full_go_test":"PASS","go_vet":"PASS","stress_passed":100,"production_touched":false,"exchange":"FakeOKX"}`, generated.Format(time.RFC3339Nano))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	newTime := time.Now().UTC().Add(-time.Hour)
	write("hermes_synthetic_lifecycle_qualification_old.json", oldTime)
	write("hermes_synthetic_lifecycle_qualification_new.json", newTime)
	got, path, hash, err := latestLifecycleQualification(dir)
	if err != nil || !got.GeneratedAt.Equal(newTime) || filepath.Base(path) != "hermes_synthetic_lifecycle_qualification_new.json" || len(hash) != 64 {
		t.Fatalf("latest selection failed: got=%+v path=%s hash=%s err=%v", got, path, hash, err)
	}
}

func TestLoadFirstOrderDryRunApprovalRejectsExpiredEvidence(t *testing.T) {
	now := time.Now().UTC()
	report := liveAutoAuditReport{GeneratedAt: now, Verdict: AuditApprovedDryRun, CurrentDryRunApproved: true, ForcedSimulation: liveguard.ForcedSimulationResult{Passed: true}}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if ok, reasons := loadFirstOrderDryRunApproval(path, now); !ok {
		t.Fatalf("fresh approval rejected: %v", reasons)
	}
	if ok, reasons := loadFirstOrderDryRunApproval(path, now.Add(24*time.Hour+time.Second)); ok || len(reasons) == 0 {
		t.Fatalf("expired approval accepted: %v", reasons)
	}
}
