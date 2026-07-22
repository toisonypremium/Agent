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

func runCanaryReadiness(cfg config.Config, db *storage.DB) error {
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
		doctor.Blockers = []string{"live doctor report unavailable"}
	}
	var reconcile liveguard.ReconcileResult
	reconcileOK := readJSONReport(filepath.Join("reports", "live_reconcile_latest.json"), &reconcile) == nil
	if !reconcileOK {
		reconcile.Safety.Status = liveguard.ReconcileBlock
		reconcile.Safety.Blockers = []string{"reconcile report unavailable"}
	}
	halted, haltErr := db.IsHalted()
	demoted, demoteErr := db.IsHermesDemoted()
	open, openErr := db.OpenLiveOrdersDetailed()
	owned, ownedErr := db.HermesOwnedPositions()
	input := liveguard.CanaryReadinessInput{QualificationPassed: qualificationOK, QualificationStressPassed: qualification.StressPassed, QualificationStressRequired: 100, QualificationArtifact: qualificationPath, QualificationSHA256: qualificationHash, QualificationAge: qualificationAge, QualificationMaxAge: 30 * 24 * time.Hour, Doctor: doctor, Reconcile: reconcile.Safety, OperatorHalted: halted, HermesDemoted: demoted, ExecutionAuthority: cfg.HermesOperator.CanExecute(), OpenLiveOrders: len(open), HermesOwnedPositions: len(owned)}
	result := liveguard.EvaluateCanaryReadiness(input)
	for label, err := range map[string]error{"operator halt": haltErr, "Hermes demotion": demoteErr, "open live orders": openErr, "Hermes-owned positions": ownedErr} {
		if err != nil {
			result.Blockers = append(result.Blockers, label+" unavailable")
			result.Verdict = liveguard.CanaryBlocked
		}
	}
	if !qualificationOK {
		reason := "qualification report unavailable or invalid"
		if qualificationErr != nil {
			reason += ": " + qualificationErr.Error()
		}
		result.Blockers = append(result.Blockers, reason)
	}
	if !doctorOK {
		result.Blockers = append(result.Blockers, "runtime doctor report unavailable")
	}
	if !reconcileOK {
		result.Blockers = append(result.Blockers, "reconcile report unavailable")
	}
	result.Summary = fmt.Sprintf("%s: blockers=%d qualification=%v stress=%d/%d doctor=%s reconcile=%s", result.Verdict, len(result.Blockers), result.QualificationPassed, result.StressPassed, result.StressRequired, result.DoctorStatus, result.ReconcileStatus)
	if err := saveJSONFile("reports", "hermes_canary_readiness_latest.json", result); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(result, "", "  ")
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
	if len(paths) == 0 {
		return lifecycleQualificationFile{}, "", "", fmt.Errorf("no lifecycle qualification artifact found")
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
