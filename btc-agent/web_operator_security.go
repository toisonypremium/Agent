package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"btc-agent/internal/storage"
)

const (
	webOperatorEmail     = "llthtt1@gmail.com"
	webCSRFCookie        = "btc_agent_csrf"
	webCSRFHeader        = "X-CSRF-Token"
	webIdentityHeader    = "Cf-Access-Authenticated-User-Email"
	webRateLimitWindow   = 5 * time.Minute
	webRateLimitAttempts = 3
)

type webOperatorSecurity struct {
	csrfToken string
	mu        sync.Mutex
	attempts  map[string][]time.Time
	now       func() time.Time
}

func newWebOperatorSecurity() (*webOperatorSecurity, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return &webOperatorSecurity{
		csrfToken: base64.RawURLEncoding.EncodeToString(buf),
		attempts:  map[string][]time.Time{},
		now:       func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *webOperatorSecurity) session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "phương thức không được hỗ trợ", http.StatusMethodNotAllowed)
		return
	}
	identity, ok := webOperatorIdentity(r)
	if !ok {
		http.Error(w, "không có quyền điều khiển", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: webCSRFCookie, Value: s.csrfToken, Path: "/api/operator/", MaxAge: 3600, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeWebJSON(w, map[string]any{"csrf_token": s.csrfToken, "operator": identity, "expires_in_seconds": 3600})
}

func (s *webOperatorSecurity) halt(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "phương thức không được hỗ trợ", http.StatusMethodNotAllowed)
			return
		}
		identity, ok := webOperatorIdentity(r)
		if !ok {
			http.Error(w, "không có quyền điều khiển", http.StatusForbidden)
			return
		}
		if !s.allow(identity) {
			w.Header().Set("Retry-After", "300")
			http.Error(w, "quá nhiều yêu cầu", http.StatusTooManyRequests)
			return
		}
		cookie, err := r.Cookie(webCSRFCookie)
		if err != nil || !secureEqual(cookie.Value, s.csrfToken) || !secureEqual(r.Header.Get(webCSRFHeader), s.csrfToken) {
			http.Error(w, "xác thực CSRF không hợp lệ", http.StatusForbidden)
			return
		}
		if r.Header.Get("X-Operator-Confirm") != "HALT" {
			http.Error(w, "cần xác nhận rõ ràng", http.StatusBadRequest)
			return
		}
		reason := strings.TrimSpace(r.Header.Get("X-Operator-Reason"))
		if len(reason) < 8 || len(reason) > 500 {
			http.Error(w, "lý do phải từ 8 đến 500 ký tự", http.StatusBadRequest)
			return
		}
		now := s.now()
		payload, _ := json.Marshal(map[string]string{"reason": reason, "operator": identity, "access_identity_header": webIdentityHeader})
		event := storage.RuntimeEvent{Timestamp: now, Source: "web-control-plane", Type: "operator_halt_request", Severity: "critical", Fingerprint: "web-halt:" + identity + ":" + now.Format("20060102150405"), PayloadJSON: string(payload)}
		if err := db.SaveRuntimeEvent(event); err != nil {
			http.Error(w, "không ghi được nhật ký", http.StatusInternalServerError)
			return
		}
		if err := db.SetHaltStatus(true); err != nil {
			http.Error(w, "không thể dừng giao dịch", http.StatusInternalServerError)
			return
		}
		writeWebJSON(w, map[string]any{"status": "HALTED", "reason": reason, "operator": identity, "at": now})
	}
}

func webOperatorIdentity(r *http.Request) (string, bool) {
	identity := strings.ToLower(strings.TrimSpace(r.Header.Get(webIdentityHeader)))
	return identity, identity == webOperatorEmail
}

func (s *webOperatorSecurity) allow(identity string) bool {
	now := s.now()
	cutoff := now.Add(-webRateLimitWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.attempts[identity][:0]
	for _, attempt := range s.attempts[identity] {
		if attempt.After(cutoff) {
			kept = append(kept, attempt)
		}
	}
	if len(kept) >= webRateLimitAttempts {
		s.attempts[identity] = kept
		return false
	}
	s.attempts[identity] = append(kept, now)
	return true
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
