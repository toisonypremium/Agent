package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

type LatestReport struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Analysis    agent1.MarketAnalysis `json:"analysis"`
	Plan        agent2.Plan           `json:"plan"`
}

func SaveReportFiles(dir string, analysis agent1.MarketAnalysis, plan agent2.Plan, markdown string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "latest.md"), []byte(markdown), 0600); err != nil {
		return err
	}
	payload := LatestReport{GeneratedAt: time.Now(), Analysis: analysis, Plan: plan}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), b, 0600)
}
