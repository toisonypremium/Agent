package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

func TestBuildWebSnapshot(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "web-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := buildWebSnapshot(config.Config{}, db)
	if s.ServiceStatus != "ACTIVE" {
		t.Fatalf("service status = %q", s.ServiceStatus)
	}
	if s.OpenOrders != 0 {
		t.Fatalf("open orders = %d", s.OpenOrders)
	}
	if !s.Halted || s.Authority != "HALTED" {
		t.Fatalf("expected fail-closed initial state, halted=%v authority=%q", s.Halted, s.Authority)
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
	for _, name := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
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
