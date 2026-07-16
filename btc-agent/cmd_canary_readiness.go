package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

type lifecycleQualificationFile struct {
	GeneratedAt  time.Time `json:"generated_at"`
	Result       string    `json:"result"`
	StressPassed int       `json:"stress_passed"`
}

func runCanaryReadiness(cfg config.Config, db *storage.DB) error {
	var qualification lifecycleQualificationFile
	qualificationOK := readJSONReport(filepath.Join("reports", "hermes_synthetic_lifecycle_qualification_20260716.json"), &qualification) == nil
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
	input := liveguard.CanaryReadinessInput{QualificationPassed: qualificationOK && qualification.Result == "PASS", QualificationStressPassed: qualification.StressPassed, QualificationStressRequired: 100, Doctor: doctor, Reconcile: reconcile.Safety, OperatorHalted: halted, HermesDemoted: demoted, ExecutionAuthority: cfg.HermesOperator.CanExecute(), OpenLiveOrders: len(open), HermesOwnedPositions: len(owned)}
	result := liveguard.EvaluateCanaryReadiness(input)
	for label, err := range map[string]error{"operator halt": haltErr, "Hermes demotion": demoteErr, "open live orders": openErr, "Hermes-owned positions": ownedErr} {
		if err != nil {
			result.Blockers = append(result.Blockers, label+" unavailable")
			result.Verdict = liveguard.CanaryBlocked
		}
	}
	if !qualificationOK {
		result.Blockers = append(result.Blockers, "qualification report unavailable")
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
