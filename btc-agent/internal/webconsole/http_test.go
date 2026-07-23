package webconsole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/storage"
)

func testAPI(t *testing.T) *API {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "web-console.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 7, 23, 22, 0, 0, 0, time.UTC)
	if err := db.SaveOrders([]agent2.PaperOrder{{ID: "paper-1", Timestamp: now.Add(-time.Hour), Symbol: "ETHUSDT", Side: "BUY", Layer: 1, Price: 1, Quantity: 2, Notional: 2, Status: "OPEN", ExpiresAt: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveRuntimeEvent(storage.RuntimeEvent{Source: "fixture", Type: "SAFE_EVENT", Severity: "info", PayloadJSON: `{"token":"must-not-leak"}`}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.AcquireExecutionLease(context.Background(), "okx-live", "fixture", now, time.Minute); err != nil || !ok {
		t.Fatalf("lease ok=%v err=%v", ok, err)
	}
	service, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return NewAPI(service, func() time.Time { return now })
}
func TestReadOnlyRoutesAndSecurityHeaders(t *testing.T) {
	api := testAPI(t)
	for _, path := range []string{"/healthz", "/api/v1/overview", "/api/v1/paper/scorecard", "/api/v1/paper/orders?limit=1", "/api/v1/events?limit=1"} {
		r := httptest.NewRecorder()
		api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, r.Code, r.Body.String())
		}
		if got := r.Header().Get("Content-Security-Policy"); got == "" {
			t.Fatalf("%s missing CSP", path)
		}
		if r.Header().Get("X-Frame-Options") != "DENY" || r.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s missing browser hardening headers", path)
		}
		if r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s missing no-store", path)
		}
	}
}
func TestOverviewIsTypedAndPaperOrdersCapped(t *testing.T) {
	api := testAPI(t)
	r := httptest.NewRecorder()
	api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	var body struct {
		SchemaVersion int `json:"schema_version"`
		Data          struct {
			Halted bool `json:"halted"`
			Paper  struct {
				Total int `json:"total_orders"`
			} `json:"paper"`
			Lease struct {
				Fresh bool `json:"fresh"`
			} `json:"lease"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SchemaVersion != SchemaVersion || !body.Data.Halted || body.Data.Paper.Total != 1 || !body.Data.Lease.Fresh {
		t.Fatalf("unexpected overview %+v", body)
	}
	r = httptest.NewRecorder()
	api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/paper/orders?limit=1000", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("cap status=%d", r.Code)
	}
	r = httptest.NewRecorder()
	api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/paper/orders?limit=bad", nil))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status=%d", r.Code)
	}
}
func TestNoMutationRouteExists(t *testing.T) {
	api := testAPI(t)
	for _, path := range []string{"/api/v1/operator/resume", "/api/v1/orders", "/api/v1/config", "/api/v1/shell"} {
		r := httptest.NewRecorder()
		api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodPost, path, nil))
		if r.Code != http.StatusNotFound {
			t.Fatalf("forbidden route %s got=%d", path, r.Code)
		}
	}
}

func TestEventsNeverExposeRuntimePayload(t *testing.T) {
	api := testAPI(t)
	r := httptest.NewRecorder()
	api.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("events status=%d", r.Code)
	}
	if strings.Contains(r.Body.String(), "must-not-leak") || strings.Contains(r.Body.String(), "payload_json") {
		t.Fatalf("unsafe event data exposed: %s", r.Body.String())
	}
}

func TestAppDoesNotServeStaticFilesWithoutExplicitDirectory(t *testing.T) {
	api := testAPI(t)
	r := httptest.NewRecorder()
	api.App("").ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("root status=%d", r.Code)
	}
}
