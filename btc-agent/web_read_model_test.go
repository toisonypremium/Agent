package main

import (
	"btc-agent/internal/circuitresearch"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeDomainStatusFailClosed(t *testing.T) {
	for in, w := range map[string]string{"MỚI": "FRESH", "CŨ": "STALE", "THIẾU": "MISSING", "LỖI": "ERROR", "": "UNKNOWN"} {
		if g := normalizeDomainStatus(in); g != w {
			t.Fatalf("%s=%s", in, g)
		}
	}
}
func TestCircuitProjectionRejectsAuthorityViolation(t *testing.T) {
	old, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(old)
	os.MkdirAll("reports/circuit", 0700)
	e := circuitresearch.Evidence{SchemaVersion: circuitresearch.EvidenceSchemaVersion, GeneratedAt: time.Now().UTC(), ValidUntil: time.Now().Add(time.Minute), ResearchAction: circuitresearch.ActionNoTrade, Authority: "EXECUTE", ExecutionIntent: nil}
	b, _ := json.Marshal(e)
	os.WriteFile(filepath.Join("reports", "circuit", "evidence_latest.json"), b, 0600)
	d := loadCircuitDashboardDomain(time.Now().UTC())
	if d.Status != "ERROR" {
		t.Fatalf("%+v", d)
	}
}
func TestCircuitProjectionAcceptsResearchOnlyShape(t *testing.T) {
	old, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(old)
	os.MkdirAll("reports/circuit", 0700)
	n := time.Now().UTC()
	e := circuitresearch.Evidence{SchemaVersion: circuitresearch.EvidenceSchemaVersion, GeneratedAt: n, ValidUntil: n.Add(time.Minute), ResearchAction: circuitresearch.ActionWatch, Authority: "RESEARCH_ONLY", ExecutionIntent: nil}
	b, _ := json.Marshal(e)
	os.WriteFile(filepath.Join("reports", "circuit", "evidence_latest.json"), b, 0600)
	d := loadCircuitDashboardDomain(n)
	if d.Status != "FRESH" {
		t.Fatalf("%+v", d)
	}
}
