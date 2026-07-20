package main

import (
	"btc-agent/internal/storage"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testWebSecurity(t *testing.T) *webOperatorSecurity {
	t.Helper()
	s, e := newWebOperatorSecurity()
	if e != nil {
		t.Fatal(e)
	}
	s.verifyIdentity = func(r *http.Request) (string, bool) {
		i := strings.ToLower(strings.TrimSpace(r.Header.Get(webIdentityHeader)))
		return i, i == webOperatorEmail
	}
	return s
}
func sessionFor(t *testing.T, s *webOperatorSecurity) (string, *http.Cookie) {
	t.Helper()
	q := httptest.NewRequest("GET", "/api/operator/session", nil)
	q.Header.Set(webIdentityHeader, webOperatorEmail)
	w := httptest.NewRecorder()
	s.session(w, q)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	return sCookie(w.Result()).Value, sCookie(w.Result())
}
func sCookie(r *http.Response) *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == webCSRFCookie {
			return c
		}
	}
	return nil
}
func haltReq(s *webOperatorSecurity, token string, c *http.Cookie, key string) *http.Request {
	r := httptest.NewRequest("POST", "/api/operator/halt", nil)
	r.Header.Set(webIdentityHeader, webOperatorEmail)
	r.Header.Set(webCSRFHeader, token)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Operator-Confirm", "HALT")
	r.Header.Set("X-Operator-Reason", "Kiểm thử dừng khẩn cấp")
	r.AddCookie(c)
	return r
}
func TestSessionRequiresIdentity(t *testing.T) {
	s := testWebSecurity(t)
	w := httptest.NewRecorder()
	s.session(w, httptest.NewRequest("GET", "/api/operator/session", nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestHaltRejectsReplayAndAudits(t *testing.T) {
	db, e := storage.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := testWebSecurity(t)
	token, c := sessionFor(t, s)
	h := s.halt(db)
	key := "0123456789abcdef"
	w := httptest.NewRecorder()
	h(w, haltReq(s, token, c, key))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h(w, haltReq(s, token, c, key))
	if w.Code != 403 {
		t.Fatal("replay accepted")
	}
	halted, _ := db.IsHalted()
	if !halted {
		t.Fatal("not halted")
	}
}
func TestHaltNeedsSessionAndIdempotency(t *testing.T) {
	db, _ := storage.Open(filepath.Join(t.TempDir(), "db"))
	defer db.Close()
	s := testWebSecurity(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/operator/halt", nil)
	r.Header.Set(webIdentityHeader, webOperatorEmail)
	s.halt(db)(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
