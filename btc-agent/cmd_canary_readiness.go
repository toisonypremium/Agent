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
	var qualification lifecycleQualificationFile
	qualificationOK := readJSONReport(filepath.Join("reports", "hermes_synthetic_lifecycle_qualification_20260716.json"), &qualification) == nil
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
	legacy = liveguard.EvaluateCanaryReadiness(liveguard.CanaryReadinessInput{QualificationPassed: qualificationOK && qualification.Result == "PASS", QualificationStressPassed: qualification.StressPassed, QualificationStressRequired: 100, Doctor: doctor, Reconcile: reconcile.Safety, OperatorHalted: halted, HermesDemoted: demoted, ExecutionAuthority: cfg.HermesOperator.CanExecute(), OpenLiveOrders: len(open), HermesOwnedPositions: len(owned)})
	var rehearsal canaryRehearsalReport
	rehearsalOK := readJSONReport(filepath.Join("reports", "canary_rehearsal_latest.json"), &rehearsal) == nil && rehearsal.Status == "PASS" && rehearsal.NoRealExchange && rehearsal.ForcedSimulation.ExchangeCalls == 0
	technical := rehearsalOK && doctorOK && doctor.Status == liveguard.DoctorOK && reconcileOK && reconcile.Safety.Status == liveguard.ReconcileClean && !halted && !demoted && len(open) == 0 && len(owned) == 0 && haltErr == nil && demoteErr == nil && openErr == nil && ownedErr == nil
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
	out.Summary = fmt.Sprintf("%s: technical=%v market_authority=%s rehearsal=%s exchange_calls=%d", out.Verdict, out.TechnicalCanaryReady, out.MarketAuthority, out.RehearsalStatus, out.RehearsalExchangeCalls)
	if err := saveJSONFile("reports", "canary_readiness_latest.json", out); err != nil {
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
