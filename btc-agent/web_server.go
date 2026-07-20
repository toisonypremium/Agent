package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

type webSnapshot struct {
	GeneratedAt     time.Time            `json:"generated_at"`
	Version         string               `json:"version"`
	ServiceStatus   string               `json:"service_status"`
	ServiceMeta     string               `json:"service_meta"`
	OpenOrders      int                  `json:"open_orders"`
	HeartbeatAge    string               `json:"heartbeat_age,omitempty"`
	Authority       string               `json:"authority"`
	AuthorityReason string               `json:"authority_reason"`
	Halted          bool                 `json:"halted"`
	Snapshot        ControlPlaneSnapshot `json:"snapshot"`
}

func runWeb(ctx context.Context, cfg config.Config, db *storage.DB, args []string) error {
	listen := argValue(args, "--listen")
	if listen == "" {
		listen = "127.0.0.1:20129"
	}
	mux := http.NewServeMux()
	registerWebDataAPI(mux, cfg, db)
	// Operator mutation controls remain fail-closed unless explicitly enabled.
	// HALT is the only Phase 2 capability and has no direct order path.
	if os.Getenv("BTC_AGENT_WEB_HALT_ENABLED") == "true" {
		security, err := newWebOperatorSecurity()
		if err != nil {
			return fmt.Errorf("initialize web operator security: %w", err)
		}
		mux.HandleFunc("/api/operator/session", security.session)
		mux.HandleFunc("/api/operator/halt", security.halt(db))
	}
	mux.HandleFunc("/api/v3/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload := buildDashboardV3(cfg, db)
		etag := `"` + payload.SnapshotID + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeWebJSON(w, payload)
	})
	mux.HandleFunc("/api/health", webHealth)
	mux.HandleFunc("/api/health/live", webHealth)
	mux.HandleFunc("/api/health/ready", func(w http.ResponseWriter, r *http.Request) { webReadiness(w, r, cfg, db) })
	mux.HandleFunc("/api/health/safety", func(w http.ResponseWriter, r *http.Request) { webSafety(w, r, cfg, db) })
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeWebJSON(w, buildWebSnapshot(cfg, db))
	})
	for _, asset := range []string{"app.js", "api.js", "state.js", "render.js", "charts.js", "index.css", "icons.svg", "test_dashboard_state.js", "test_dashboard_escape.js", "test_dashboard_contract.js"} {
		asset := asset
		mux.HandleFunc("/web/"+asset, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
			http.ServeFile(w, r, filepath.Join("web", asset))
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join("web", "index.html"))
	})
	server := &http.Server{Addr: listen, Handler: webSecurityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Printf("btc-agent web listening on %s\n", listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func buildWebSnapshot(cfg config.Config, db *storage.DB) webSnapshot {
	s := buildControlPlaneSnapshot(cfg, db)
	openOrders := 0
	if orders, err := db.OpenLiveOrdersDetailed(); err == nil {
		openOrders = len(orders)
	}
	out := webSnapshot{
		GeneratedAt:     time.Now().UTC(),
		Version:         version,
		ServiceStatus:   "UNKNOWN",
		ServiceMeta:     "scheduler evidence unavailable",
		OpenOrders:      openOrders,
		Authority:       "BLOCKED",
		AuthorityReason: "production safety evidence incomplete",
		Halted:          s.Operator.Halted,
		Snapshot:        s,
	}
	if s.Operator.Halted {
		out.ServiceStatus = "HALTED"
		out.Authority = "HALTED"
		out.AuthorityReason = "operator halt active"
		return out
	}

	heartbeat := loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute)
	heartbeatState := webReportString(heartbeat, "status")
	if !heartbeat.CapNhatLuc.IsZero() {
		out.HeartbeatAge = time.Since(heartbeat.CapNhatLuc).Round(time.Second).String()
	}
	out.ServiceMeta = heartbeatState
	if heartbeat.TrangThai == "MỚI" && (heartbeatState == "running" || heartbeatState == "healthy") {
		out.ServiceStatus = "ACTIVE"
	} else {
		out.ServiceStatus = "DEGRADED"
		out.AuthorityReason = "scheduler is not ready"
		return out
	}

	doctor := loadWebReport("live_doctor_latest.json", 35*time.Minute)
	if doctor.TrangThai != "MỚI" || webReportString(doctor, "status") != "DOCTOR_OK" {
		out.AuthorityReason = "runtime doctor is not fresh and healthy"
		return out
	}
	reconcile := loadWebReport("live_reconcile_latest.json", 35*time.Minute)
	if reconcile.TrangThai != "MỚI" || webReportNestedString(reconcile, "safety", "status") != "RECONCILE_CLEAN" {
		out.AuthorityReason = "reconciliation is not fresh and clean"
		return out
	}
	analysis, analysisOK := s.Market.(agent1.MarketAnalysis)
	plan, planOK := s.Plan.(agent2.Plan)
	if !analysisOK || !planOK || analysis.Timestamp.IsZero() || plan.Timestamp.IsZero() {
		out.AuthorityReason = "current market analysis or execution plan is unavailable"
		return out
	}
	now := time.Now().UTC()
	if analysis.Timestamp.After(now.Add(time.Minute)) || plan.Timestamp.After(now.Add(time.Minute)) || now.Sub(analysis.Timestamp) > 35*time.Minute || now.Sub(plan.Timestamp) > 35*time.Minute {
		out.AuthorityReason = "current market analysis or execution plan is stale"
		return out
	}
	if analysis.ActionPermission != agent1.Allowed || analysis.BTCAccumulation.Phase != accumulation.PhaseConfirmed || plan.State != agent2.StateActiveLimit || plan.ActionPermission != agent1.Allowed {
		out.AuthorityReason = fmt.Sprintf("deterministic gates blocked: permission=%s accumulation=%s plan=%s", analysis.ActionPermission, analysis.BTCAccumulation.Phase, plan.State)
		return out
	}
	out.Authority = "ALLOWED"
	out.AuthorityReason = "fresh scheduler, doctor, reconcile, analysis and deterministic execution plan agree"
	return out
}

func webHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeWebJSON(w, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func writeWebJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func webSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; upgrade-insecure-requests")
		next.ServeHTTP(w, r)
	})
}

func webReadiness(w http.ResponseWriter, r *http.Request, cfg config.Config, db *storage.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := "ready"
	checks := map[string]string{}
	if _, err := db.LivePositions(); err != nil {
		status = "not_ready"
		checks["sqlite"] = "error"
	} else {
		checks["sqlite"] = "ok"
	}
	heartbeat := loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute)
	heartbeatState := webReportString(heartbeat, "status")
	checks["scheduler_report"] = heartbeat.TrangThai
	checks["scheduler_state"] = heartbeatState
	if heartbeat.TrangThai != "MỚI" || (heartbeatState != "running" && heartbeatState != "healthy") {
		status = "not_ready"
	}
	doctor := loadWebReport("live_doctor_latest.json", 35*time.Minute)
	doctorState := webReportString(doctor, "status")
	checks["doctor_report"] = doctor.TrangThai
	checks["doctor_state"] = doctorState
	if doctor.TrangThai != "MỚI" || doctorState != "DOCTOR_OK" {
		status = "not_ready"
	}
	writeWebJSON(w, map[string]any{"status": status, "checks": checks, "time": time.Now().UTC()})
}

func webSafety(w http.ResponseWriter, r *http.Request, cfg config.Config, db *storage.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot := buildWebSnapshot(cfg, db)
	status := "blocked"
	if snapshot.Halted {
		status = "halted"
	}
	writeWebJSON(w, map[string]any{"status": status, "authority": snapshot.Authority, "real_order": "NOT_APPROVED", "halted": snapshot.Halted, "time": time.Now().UTC()})
}

func webReportString(report webReport, key string) string {
	object, ok := report.DuLieu.(map[string]any)
	if !ok {
		return "unknown"
	}
	value, ok := object[key].(string)
	if !ok || value == "" {
		return "unknown"
	}
	return value
}

func webReportNestedString(report webReport, objectKey, valueKey string) string {
	object, ok := report.DuLieu.(map[string]any)
	if !ok {
		return "unknown"
	}
	nested, ok := object[objectKey].(map[string]any)
	if !ok {
		return "unknown"
	}
	value, ok := nested[valueKey].(string)
	if !ok || value == "" {
		return "unknown"
	}
	return value
}
