package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

func TestBuildWebSnapshotFailsClosedWhenHalted(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "web-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := buildWebSnapshot(config.Config{}, db)
	if s.ServiceStatus != "HALTED" {
		t.Fatalf("service status = %q", s.ServiceStatus)
	}
	if s.OpenOrders != 0 {
		t.Fatalf("open orders = %d", s.OpenOrders)
	}
	if !s.Halted || s.Authority != "HALTED" || s.AuthorityReason == "" {
		t.Fatalf("expected fail-closed initial state, halted=%v authority=%q reason=%q", s.Halted, s.Authority, s.AuthorityReason)
	}
}

func TestBuildWebSnapshotMissingEvidenceIsDegradedAndBlocked(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	db, err := storage.Open(filepath.Join(t.TempDir(), "web-degraded.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	s := buildWebSnapshot(config.Config{}, db)
	if s.Halted || s.ServiceStatus != "DEGRADED" || s.Authority != "BLOCKED" {
		t.Fatalf("halted=%v service=%q authority=%q reason=%q", s.Halted, s.ServiceStatus, s.Authority, s.AuthorityReason)
	}
	if s.AuthorityReason == "" {
		t.Fatal("missing fail-closed authority reason")
	}
}

func TestWebHealthAndSecurityHeaders(t *testing.T) {
	handler := webSecurityHeaders(http.HandlerFunc(webHealth))
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	for _, name := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy", "Cross-Origin-Opener-Policy", "Cross-Origin-Resource-Policy"} {
		if res.Header().Get(name) == "" {
			t.Errorf("missing %s", name)
		}
	}
}

func TestWebHealthRejectsPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	res := httptest.NewRecorder()
	webHealth(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestSummarizeWebPayloadUsesWorstReport(t *testing.T) {
	payload := map[string]any{
		"fresh":   webReport{TrangThai: "MỚI", DoCuGiay: 2, Nguon: "fresh.json"},
		"stale":   webReport{TrangThai: "CŨ", DoCuGiay: 99, Nguon: "stale.json"},
		"missing": webReport{TrangThai: "THIẾU", Nguon: "missing.json"},
	}
	status, age, warnings := summarizeWebPayload(payload)
	if status != "THIẾU" || age != 99 || len(warnings) != 2 {
		t.Fatalf("status=%q age=%d warnings=%v", status, age, warnings)
	}
}

func TestWebSafetyFailsClosed(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "web-safety.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res := httptest.NewRecorder()
	webSafety(res, httptest.NewRequest(http.MethodGet, "/api/health/safety", nil), config.Config{}, db)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["real_order"] != "NOT_APPROVED" || payload["authority"] != "HALTED" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestWebReportRejectsMalformedExplicitTimestamp(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "reports"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reports", "bad.json"), []byte(`{"generated_at":"not-a-time"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	r := loadWebReport("bad.json", time.Hour)
	if r.TrangThai != "LỖI" {
		t.Fatalf("status=%q", r.TrangThai)
	}
}

func TestWebReportRejectsFutureTimestamp(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "reports"), 0700); err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339Nano)
	if err := os.WriteFile(filepath.Join(dir, "reports", "future.json"), []byte(`{"generated_at":"`+future+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	r := loadWebReport("future.json", time.Hour)
	if r.TrangThai != "LỖI" {
		t.Fatalf("status=%q", r.TrangThai)
	}
}
