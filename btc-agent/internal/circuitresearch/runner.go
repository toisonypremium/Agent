package circuitresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"btc-agent/internal/reportio"
)

type RunnerConfig struct {
	Python         string
	Adapter        string
	ProducerCommit string
	OutputDir      string
	Timeout        time.Duration
}

// RunDeterministic executes the no-network adapter, validates the candidate,
// and promotes only a valid report. A failed run never overwrites latest valid evidence.
func RunDeterministic(ctx context.Context, input InputSnapshot, cfg RunnerConfig) (ValidationResult, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.OutputDir == "" {
		return ValidationResult{}, fmt.Errorf("output directory required")
	}
	if err := reportio.EnsureDir(cfg.OutputDir); err != nil {
		return ValidationResult{}, err
	}
	runDir, err := os.MkdirTemp(cfg.OutputDir, ".run-*")
	if err != nil {
		return ValidationResult{}, err
	}
	defer os.RemoveAll(runDir)
	inputPath := filepath.Join(runDir, "input.json")
	candidatePath := filepath.Join(runDir, "evidence.json")
	rawInput, err := json.Marshal(input)
	if err != nil {
		return ValidationResult{}, err
	}
	if err := os.WriteFile(inputPath, rawInput, 0600); err != nil {
		return ValidationResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, cfg.Python, cfg.Adapter, "--input", inputPath, "--output", candidatePath, "--producer-commit", cfg.ProducerCommit)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + runDir, "PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1"}
	output, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return ValidationResult{}, fmt.Errorf("CIRCUIT_RUN_TIMEOUT")
	}
	if err != nil {
		return ValidationResult{}, fmt.Errorf("CIRCUIT_RUN_FAILED: %w: %.1024s", err, output)
	}
	rawEvidence, err := os.ReadFile(candidatePath)
	if err != nil {
		return ValidationResult{}, err
	}
	result := DecodeAndValidateEvidence(rawEvidence, ValidationPolicy{Now: time.Now().UTC(), ExpectedInput: input, AllowedProducerCommit: cfg.ProducerCommit, MaxTTL: 15 * time.Minute})
	if !result.Valid {
		return result, fmt.Errorf("CIRCUIT_EVIDENCE_REJECTED")
	}
	if err := reportio.WriteJSON(cfg.OutputDir, "input_latest.json", input); err != nil {
		return result, err
	}
	if err := reportio.WriteJSON(cfg.OutputDir, "evidence_latest.json", result.Evidence); err != nil {
		return result, err
	}
	return result, nil
}
