package webconsole

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	service *Service
	now     Clock
}

func NewAPI(service *Service, now Clock) *API {
	if now == nil {
		now = time.Now
	}
	return &API{service: service, now: now}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/overview", a.overview)
	mux.HandleFunc("GET /api/v1/paper/scorecard", a.scorecard)
	mux.HandleFunc("GET /api/v1/paper/orders", a.paperOrders)
	mux.HandleFunc("GET /api/v1/events", a.events)
	return secureHeaders(mux)
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (a *API) overview(w http.ResponseWriter, r *http.Request) {
	out, err := a.service.Overview(r.Context())
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "overview_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) scorecard(w http.ResponseWriter, _ *http.Request) {
	out, err := a.service.Scorecard()
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "paper_scorecard_unavailable")
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
		writeProblem(w, http.StatusServiceUnavailable, "events_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}
func (a *API) paperOrders(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeProblem(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = parsed
	}
	out, err := a.service.PaperOrders(limit)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "paper_orders_unavailable")
		return
	}
	writeEnvelope(w, a.now, out)
}

func queryLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeProblem(w, http.StatusBadRequest, "invalid_limit")
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}
func writeEnvelope[T any](w http.ResponseWriter, now Clock, data T) {
	writeJSON(w, http.StatusOK, Envelope[T]{SchemaVersion: SchemaVersion, GeneratedAt: now().UTC(), Freshness: Freshness{State: "fresh", AgeSeconds: 0}, Data: data, Warnings: []string{}})
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
