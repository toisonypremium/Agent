package main

import (
	"btc-agent/internal/storage"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	webOperatorEmail  = "llthtt1@gmail.com"
	webCSRFCookie     = "btc_agent_csrf"
	webCSRFHeader     = "X-CSRF-Token"
	webIdentityHeader = "Cf-Access-Authenticated-User-Email"
)

type webOperatorSession struct {
	Token, Identity string
	ExpiresAt       time.Time
	Used            map[string]time.Time
}
type webOperatorSecurity struct {
	mu             sync.Mutex
	sessions       map[string]*webOperatorSession
	now            func() time.Time
	accessJWT      *cloudflareAccessVerifier
	verifyIdentity func(*http.Request) (string, bool)
}

func newWebOperatorSecurity() (*webOperatorSecurity, error) {
	return &webOperatorSecurity{sessions: map[string]*webOperatorSession{}, now: func() time.Time { return time.Now().UTC() }, accessJWT: newCloudflareAccessVerifier()}, nil
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *webOperatorSecurity) session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "phương thức không được hỗ trợ", 405)
		return
	}
	identity, ok := s.webOperatorIdentity(r)
	if !ok {
		http.Error(w, "không có quyền điều khiển", 403)
		return
	}
	token, e := randomToken()
	if e != nil {
		http.Error(w, "không tạo được phiên", 500)
		return
	}
	expires := s.now().Add(time.Hour)
	s.mu.Lock()
	s.sessions[token] = &webOperatorSession{Token: token, Identity: identity, ExpiresAt: expires, Used: map[string]time.Time{}}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: webCSRFCookie, Value: token, Path: "/api/operator/", MaxAge: 3600, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeWebJSON(w, map[string]any{"csrf_token": token, "operator": identity, "expires_at": expires, "capabilities": []string{}})
}
func (s *webOperatorSecurity) authorize(r *http.Request) (string, error) {
	identity, ok := s.webOperatorIdentity(r)
	if !ok {
		return "", http.ErrNoCookie
	}
	cookie, e := r.Cookie(webCSRFCookie)
	if e != nil || !secureEqual(cookie.Value, r.Header.Get(webCSRFHeader)) {
		return "", http.ErrNoCookie
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[cookie.Value]
	if !ok || s.now().After(session.ExpiresAt) || !secureEqual(session.Identity, identity) {
		return "", http.ErrNoCookie
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 128 {
		return "", http.ErrNotSupported
	}
	if _, used := session.Used[key]; used {
		return "", http.ErrUseLastResponse
	}
	session.Used[key] = s.now()
	return identity, nil
}
func (s *webOperatorSecurity) halt(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "phương thức không được hỗ trợ", 405)
			return
		}
		identity, e := s.authorize(r)
		if e != nil {
			http.Error(w, "xác thực operator, CSRF hoặc idempotency không hợp lệ", 403)
			return
		}
		if r.Header.Get("X-Operator-Confirm") != "HALT" {
			http.Error(w, "cần xác nhận rõ ràng", 400)
			return
		}
		reason := strings.TrimSpace(r.Header.Get("X-Operator-Reason"))
		if len(reason) < 8 || len(reason) > 500 {
			http.Error(w, "lý do phải từ 8 đến 500 ký tự", 400)
			return
		}
		now := s.now()
		payload, _ := json.Marshal(map[string]string{"reason": reason, "operator": identity, "identity_authority": "verified_cloudflare_access_jwt"})
		event := storage.RuntimeEvent{Timestamp: now, Source: "web-control-plane", Type: "operator_halt_request", Severity: "critical", Fingerprint: "web-halt:" + identity + ":" + now.Format("20060102150405.000000000"), PayloadJSON: string(payload)}
		if e = db.SaveRuntimeEvent(event); e != nil {
			http.Error(w, "không ghi được nhật ký", 500)
			return
		}
		if e = db.SetHaltStatus(true); e != nil {
			http.Error(w, "không thể dừng giao dịch", 500)
			return
		}
		writeWebJSON(w, map[string]any{"status": "HALTED", "reason": reason, "operator": identity, "at": now})
	}
}
func (s *webOperatorSecurity) webOperatorIdentity(r *http.Request) (string, bool) {
	if s.verifyIdentity != nil {
		return s.verifyIdentity(r)
	}
	claims, e := s.accessJWT.verifyRequest(r)
	if e != nil {
		return "", false
	}
	identity := strings.ToLower(strings.TrimSpace(claims.Email))
	header := strings.ToLower(strings.TrimSpace(r.Header.Get(webIdentityHeader)))
	return identity, identity == webOperatorEmail && secureEqual(identity, header)
}
func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
