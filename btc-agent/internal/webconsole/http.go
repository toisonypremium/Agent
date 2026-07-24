package webconsole

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Envelope[T any] struct {
	SchemaVersion int       `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Freshness     Freshness `json:"freshness"`
	Data          T         `json:"data"`
	Warnings      []string  `json:"warnings"`
}
type API struct {
	service       *Service
	now           Clock
	access        *accessVerifier
	publicOrigin  string
	ownerIdentity string
}

func NewAPI(service *Service, now Clock) *API {
	if now == nil {
		now = time.Now
	}
	return &API{service: service, now: now}
}
func (a *API) ConfigureAccess(teamDomain, audience, publicOrigin, ownerIdentity string) error {
	v, err := newAccessVerifier(AccessConfig{TeamDomain: teamDomain, Audience: audience})
	if err != nil {
		return err
	}
	a.access = v
	a.publicOrigin = strings.TrimRight(publicOrigin, "/")
	a.ownerIdentity = strings.TrimSpace(strings.ToLower(ownerIdentity))
	return nil
}
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/csrf", a.csrf)
	mux.HandleFunc("GET /api/v1/overview", a.overview)
	mux.HandleFunc("GET /api/v1/paper/scorecard", a.scorecard)
	mux.HandleFunc("GET /api/v1/operations/health", a.runtimeHealth)
	mux.HandleFunc("GET /api/v1/capital/overview", a.capitalOverview)
	mux.HandleFunc("GET /api/v1/market-planner", a.marketPlanner)
	mux.HandleFunc("GET /api/v1/capital/theses", a.thesisCapital)
	mux.HandleFunc("GET /api/v1/paper/orders", a.paperOrders)
	mux.HandleFunc("GET /api/v1/events", a.events)
	mux.HandleFunc("GET /api/v1/audit", a.audit)
	mux.HandleFunc("GET /api/v1/reports", a.reports)
	mux.HandleFunc("POST /api/v1/halt", a.halt)
	return secureHeaders(mux)
}
func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (a *API) csrf(w http.ResponseWriter, _ *http.Request) {
	token := randomToken()
	http.SetCookie(w, &http.Cookie{Name: "btc_agent_csrf", Value: token, Path: "/", Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode, MaxAge: 900})
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}
func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	out, err := a.service.Overview(r.Context())
	if err != nil {
		writeProblem(w, 503, "overview_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) runtimeHealth(w http.ResponseWriter, _ *http.Request) {
	out, err := a.service.RuntimeHealth()
	if err != nil {
		writeProblem(w, 503, "runtime_health_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) marketPlanner(w http.ResponseWriter, _ *http.Request) {
	out, err := a.service.MarketPlanner()
	if err != nil {
		writeProblem(w, 503, "market_planner_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) capitalOverview(w http.ResponseWriter, _ *http.Request) {
	out, err := a.service.CapitalOverview()
	if err != nil {
		writeProblem(w, 503, "capital_overview_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) thesisCapital(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	out, err := a.service.ThesisCapital(limit)
	if err != nil {
		writeProblem(w, 503, "thesis_capital_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) scorecard(w http.ResponseWriter, _ *http.Request) {
	out, err := a.service.Scorecard()
	if err != nil {
		writeProblem(w, 503, "paper_scorecard_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) reports(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, a.now, a.service.Reports())
}
func (a *API) audit(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	out, err := a.service.Audit(limit, RoleViewer)
	if err != nil {
		writeProblem(w, 503, "audit_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	out, err := a.service.Events(limit)
	if err != nil {
		writeProblem(w, 503, "events_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) paperOrders(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(w, r)
	if !ok {
		return
	}
	since, err := parseSince(r.URL.Query().Get("since"))
	if err != nil {
		writeProblem(w, 400, "invalid_since")
		return
	}
	out, err := a.service.PaperOrdersFiltered(limit, r.URL.Query().Get("status"), since)
	if err != nil {
		writeProblem(w, 503, "paper_orders_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}

type haltRequest struct {
	Reason string `json:"reason"`
}

func (a *API) halt(w http.ResponseWriter, r *http.Request) {
	if a.access == nil || a.publicOrigin == "" {
		writeProblem(w, 503, "halt_unavailable")
		return
	}
	if r.Header.Get("Origin") != a.publicOrigin {
		writeProblem(w, 403, "origin_forbidden")
		return
	}
	c, e := r.Cookie("btc_agent_csrf")
	if e != nil || c.Value == "" || r.Header.Get("X-CSRF-Token") != c.Value {
		writeProblem(w, 403, "csrf_forbidden")
		return
	}
	identity, e := a.access.identity(r)
	if e != nil {
		writeProblem(w, 401, "access_identity_required")
		return
	}
	if roleForIdentity(a.ownerIdentity, identity) != RoleOperator {
		writeProblem(w, 403, "operator_role_required")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var body haltRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
		writeProblem(w, 400, "invalid_halt_request")
		return
	}
	receipt, e := a.service.RequestHalt(identity, body.Reason, key)
	if e != nil {
		writeProblem(w, 400, "halt_request_rejected")
		return
	}
	writeEnvelope(w, a.now, receipt)
}
func parseSince(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}
func queryLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeProblem(w, 400, "invalid_limit")
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}
func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func writeEnvelope[T any](w http.ResponseWriter, now Clock, data T) {
	writeJSON(w, 200, Envelope[T]{SchemaVersion: SchemaVersion, GeneratedAt: now().UTC(), Freshness: Freshness{State: "fresh", AgeSeconds: 0}, Data: data, Warnings: []string{}})
}
func writeProblem(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}
