package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"btc-agent/internal/circuitresearch"
	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

func runCircuitResearchSnapshot(cfg config.Config, db *storage.DB) error {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	plan, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	snapshot, err := circuitresearch.BuildInput(analysis, plan, circuitresearch.Producer{Name: "btc-agent", Version: "2026.07.20-remediation", Commit: "481aa4dbbe85"}, time.Now().UTC(), 15*time.Minute)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot)
}
func runCircuitResearchValidate(args []string) error {
	inputPath := argValue(args, "--input")
	evidencePath := argValue(args, "--evidence")
	producer := argValue(args, "--producer-commit")
	if inputPath == "" || evidencePath == "" || producer == "" {
		return fmt.Errorf("--input, --evidence and --producer-commit required")
	}
	inputRaw, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if len(inputRaw) > circuitresearch.MaxPayloadBytes {
		return fmt.Errorf("input exceeds 64 KiB")
	}
	var input circuitresearch.InputSnapshot
	dec := json.NewDecoder(bytes.NewReader(inputRaw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	if err := circuitresearch.ValidateInputHash(input); err != nil {
		return err
	}
	evidenceRaw, err := os.ReadFile(evidencePath)
	if err != nil {
		return err
	}
	result := circuitresearch.DecodeAndValidateEvidence(evidenceRaw, circuitresearch.ValidationPolicy{Now: time.Now().UTC(), ExpectedInput: input, AllowedProducerCommit: producer, MaxTTL: 15 * time.Minute})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
	if !result.Valid {
		return fmt.Errorf("Circuit evidence rejected")
	}
	return nil
}
