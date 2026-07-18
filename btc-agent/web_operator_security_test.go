package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"btc-agent/internal/storage"
)

func testWebSecurity(t *testing.T) *webOperatorSecurity {
	t.Helper()
	s, err := newWebOperatorSecurity()
	if err != nil {
		t.Fatal(err)
	}
	// Unit tests below target CSRF/halt semantics. JWT cryptographic validation
	// has dedicated verifier tests and production never installs this override.
	s.verifyIdentity = func(r *http.Request) (string, bool) {
		identity := strings.ToLower(strings.TrimSpace(r.Header.Get(webIdentityHeader)))
		return identity, identity == webOperatorEmail
	}
	return s
}

func TestOperatorSessionRequiresCloudflareIdentity(t *testing.T) {
	s := testWebSecurity(t)
	res := httptest.NewRecorder()
	s.session(res, httptest.NewRequest(http.MethodGet, "/api/operator/session", nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestOperatorHaltRequiresIdentityAndCSRF(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "halt-security.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := testWebSecurity(t)
	handler := s.halt(db)

	for _, tc := range []struct {
		name                    string
		identity, cookie, token string
		want                    int
	}{
		{name: "missing identity", want: http.StatusForbidden},
		{name: "wrong identity", identity: "attacker@example.com", want: http.StatusForbidden},
		{name: "missing csrf", identity: webOperatorEmail, want: http.StatusForbidden},
		{name: "wrong csrf", identity: webOperatorEmail, cookie: "wrong", token: "wrong", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/operator/halt", nil)
			if tc.identity != "" {
				req.Header.Set(webIdentityHeader, tc.identity)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: webCSRFCookie, Value: tc.cookie})
			}
			if tc.token != "" {
				req.Header.Set(webCSRFHeader, tc.token)
			}
			res := httptest.NewRecorder()
			handler(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d", res.Code, tc.want)
			}
		})
	}
}

func TestOperatorHaltAuditsIdentityAndRemainsAvailable(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "halt-audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := testWebSecurity(t)
	now := time.Date(2026, 7, 18, 16, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	handler := s.halt(db)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/operator/halt", nil)
		req.Header.Set(webIdentityHeader, webOperatorEmail)
		res := httptest.NewRecorder()
		handler(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("malformed attempt %d status=%d", i, res.Code)
		}
	}

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/operator/halt", nil)
		req.Header.Set(webIdentityHeader, strings.ToUpper(webOperatorEmail))
		req.Header.Set(webCSRFHeader, s.csrfToken)
		req.Header.Set("X-Operator-Confirm", "HALT")
		req.Header.Set("X-Operator-Reason", "Kiểm thử dừng khẩn cấp")
		req.AddCookie(&http.Cookie{Name: webCSRFCookie, Value: s.csrfToken})
		res := httptest.NewRecorder()
		handler(res, req)
		return res
	}
	// Emergency halt is idempotent and must remain available. Repeated valid
	// requests must never lock the operator out of this safety action.
	for i := 0; i < 10; i++ {
		if res := request(); res.Code != http.StatusOK {
			t.Fatalf("attempt %d status=%d body=%s", i, res.Code, res.Body.String())
		}
	}
	if halted, err := db.IsHalted(); err != nil || !halted {
		t.Fatalf("halted=%v err=%v", halted, err)
	}
	events, err := db.PendingRuntimeEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("missing audit event")
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(events[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["operator"] != webOperatorEmail {
		t.Fatalf("operator=%q", payload["operator"])
	}
}
