package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

type lifecycleQualificationFile struct {
	GeneratedAt       time.Time `json:"generated_at"`
	Qualification     string    `json:"qualification"`
	Result            string    `json:"result"`
	FullGoTest        string    `json:"full_go_test"`
	GoVet             string    `json:"go_vet"`
	StressPassed      int       `json:"stress_passed"`
	ProductionTouched bool      `json:"production_touched"`
	Exchange          string    `json:"exchange"`
}

type canaryReadinessReport struct {
	GeneratedAt             time.Time                       `json:"generated_at"`
	Verdict                 string                          `json:"verdict"`
	TechnicalCanaryReady    bool                            `json:"technical_canary_ready"`
	MarketAuthority         string                          `json:"market_authority"`
	ProductionCanaryAllowed bool                            `json:"production_canary_allowed"`
	RehearsalStatus         string                          `json:"rehearsal_status"`
	RehearsalExchangeCalls  int                             `json:"rehearsal_exchange_calls"`
	DoctorStatus            string                          `json:"doctor_status"`
	ReconcileStatus         string                          `json:"reconcile_status"`
	OpenLiveOrders          int                             `json:"open_live_orders"`
	HermesOwnedPositions    int                             `json:"hermes_owned_positions"`
	Blockers                []string                        `json:"blockers,omitempty"`
	Summary                 string                          `json:"summary"`
	Legacy                  liveguard.CanaryReadinessResult `json:"legacy_readiness"`
}

func runCanaryReadiness(cfg config.Config, db *storage.DB) error {
	var legacy liveguard.CanaryReadinessResult
	qualification, qualificationPath, qualificationHash, qualificationErr := latestLifecycleQualification("reports")
	qualificationAge := time.Duration(0)
	if !qualification.GeneratedAt.IsZero() {
		qualificationAge = time.Since(qualification.GeneratedAt)
	}
	qualificationOK := qualificationErr == nil && qualification.Qualification == "HERMES_SYNTHETIC_LIFECYCLE" && qualification.Result == "PASS" && qualification.FullGoTest == "PASS" && qualification.GoVet == "PASS" && !qualification.ProductionTouched && qualification.Exchange == "FakeOKX"
	var doctor liveguard.RuntimeDoctorResult
	doctorOK := readJSONReport(filepath.Join("reports", "live_doctor_latest.json"), &doctor) == nil
	if !doctorOK {
		doctor.Status = liveguard.DoctorBlock
	}
	var reconcile liveguard.ReconcileResult
	reconcileOK := readJSONReport(filepath.Join("reports", "live_reconcile_latest.json"), &reconcile) == nil
	if !reconcileOK {
		reconcile.Safety.Status = liveguard.ReconcileBlock
	}
	halted, haltErr := db.IsHalted()
	demoted, demoteErr := db.IsHermesDemoted()
	open, openErr := db.OpenLiveOrdersDetailed()
	owned, ownedErr := db.HermesOwnedPositions()
	legacy = liveguard.EvaluateCanaryReadiness(liveguard.CanaryReadinessInput{QualificationPassed: qualificationOK, QualificationStressPassed: qualification.StressPassed, QualificationStressRequired: 100, QualificationArtifact: qualificationPath, QualificationSHA256: qualificationHash, QualificationAge: qualificationAge, QualificationMaxAge: 30 * 24 * time.Hour, Doctor: doctor, Reconcile: reconcile.Safety, OperatorHalted: halted, HermesDemoted: demoted, ExecutionAuthority: cfg.HermesOperator.CanExecute(), OpenLiveOrders: len(open), HermesOwnedPositions: len(owned)})
	var rehearsal canaryRehearsalReport
	rehearsalOK := readJSONReport(filepath.Join("reports", "canary_rehearsal_latest.json"), &rehearsal) == nil && rehearsal.Status == "PASS" && rehearsal.NoRealExchange && rehearsal.ForcedSimulation.ExchangeCalls == 0
	// Existing managed holdings are a production-canary blocker, not a
	// technical rehearsal blocker. The rehearsal uses isolated storage and
	// already proves order/replay behavior with no exchange calls.
	technical := rehearsalOK && doctorOK && doctor.Status == liveguard.DoctorOK && reconcileOK && reconcile.Safety.Status == liveguard.ReconcileClean && !halted && !demoted && len(open) == 0 && haltErr == nil && demoteErr == nil && openErr == nil && ownedErr == nil
	marketAuthority := "BLOCKED"
	if legacy.Verdict == liveguard.CanaryReady {
		marketAuthority = "READY"
	}
	out := canaryReadinessReport{GeneratedAt: time.Now().UTC(), Verdict: "PRODUCTION_CANARY_NOT_AUTHORIZED", TechnicalCanaryReady: technical, MarketAuthority: marketAuthority, ProductionCanaryAllowed: false, RehearsalStatus: rehearsal.Status, RehearsalExchangeCalls: rehearsal.ForcedSimulation.ExchangeCalls, DoctorStatus: doctor.Status, ReconcileStatus: reconcile.Safety.Status, OpenLiveOrders: len(open), HermesOwnedPositions: len(owned), Legacy: legacy}
	if !technical {
		out.Blockers = append(out.Blockers, "technical canary rehearsal or runtime gate not ready")
	}
	if marketAuthority != "READY" {
		out.Blockers = append(out.Blockers, "market authority blocked; production canary remains unauthorized")
	}
	if len(owned) != 0 {
		out.Blockers = append(out.Blockers, fmt.Sprintf("%d Hermes-owned positions require production reconciliation before canary", len(owned)))
	}
	out.Summary = fmt.Sprintf("%s: technical=%v market_authority=%s rehearsal=%s exchange_calls=%d", out.Verdict, out.TechnicalCanaryReady, out.MarketAuthority, out.RehearsalStatus, out.RehearsalExchangeCalls)
	if err := saveJSONFile("reports", "canary_readiness_latest.json", out); err != nil {
		return err
	}
	if err := saveJSONFile("reports", "hermes_canary_readiness_latest.json", legacy); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return nil
}

func readJSONReport(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func latestLifecycleQualification(dir string) (lifecycleQualificationFile, string, string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "hermes_synthetic_lifecycle_qualification*.json"))
	if err != nil {
		return lifecycleQualificationFile{}, "", "", err
	}
	type candidate struct {
		path string
		data []byte
		file lifecycleQualificationFile
	}
	candidates := []candidate{}
	for _, path := range paths {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var q lifecycleQualificationFile
		if json.Unmarshal(b, &q) != nil || q.GeneratedAt.IsZero() {
			continue
		}
		candidates = append(candidates, candidate{path: path, data: b, file: q})
	}
	if len(candidates) == 0 {
		return lifecycleQualificationFile{}, "", "", fmt.Errorf("no valid lifecycle qualification artifact found")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].file.GeneratedAt.After(candidates[j].file.GeneratedAt) })
	selected := candidates[0]
	sum := sha256.Sum256(selected.data)
	return selected.file, selected.path, hex.EncodeToString(sum[:]), nil
}
