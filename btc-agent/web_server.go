package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

type webSnapshot struct {
	GeneratedAt   time.Time            `json:"generated_at"`
	Version       string               `json:"version"`
	ServiceStatus string               `json:"service_status"`
	ServiceMeta   string               `json:"service_meta"`
	OpenOrders    int                  `json:"open_orders"`
	HeartbeatAge  string               `json:"heartbeat_age,omitempty"`
	Authority     string               `json:"authority"`
	Halted        bool                 `json:"halted"`
	Snapshot      ControlPlaneSnapshot `json:"snapshot"`
}

func runWeb(ctx context.Context, cfg config.Config, db *storage.DB, args []string) error {
	listen := argValue(args, "--listen")
	if listen == "" {
		listen = "127.0.0.1:20129"
	}
	mux := http.NewServeMux()
	registerWebDataAPI(mux, cfg, db)
	mux.HandleFunc("/api/health", webHealth)
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeWebJSON(w, buildWebSnapshot(cfg, db))
	})
	operatorSecurity, err := newWebOperatorSecurity()
	if err != nil {
		return fmt.Errorf("initialize web operator security: %w", err)
	}
	mux.HandleFunc("/api/operator/session", operatorSecurity.session)
	mux.HandleFunc("/api/operator/halt", operatorSecurity.halt(db))
	mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
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
	out := webSnapshot{GeneratedAt: time.Now().UTC(), Version: version, ServiceStatus: "ACTIVE", ServiceMeta: "Scheduler running", OpenOrders: openOrders, Authority: "BLOCKED", Halted: s.Operator.Halted, Snapshot: s}
	if s.Operator.Halted {
		out.Authority = "HALTED"
	}
	var heartbeat struct {
		GeneratedAt string `json:"generated_at"`
		Status      string `json:"status"`
	}
	if b, err := os.ReadFile(filepath.Join("reports", "scheduler_heartbeat_latest.json")); err == nil && json.Unmarshal(b, &heartbeat) == nil {
		if heartbeat.GeneratedAt != "" {
			if t, err := time.Parse(time.RFC3339, heartbeat.GeneratedAt); err == nil {
				out.HeartbeatAge = time.Since(t).Round(time.Second).String()
			}
		}
		if heartbeat.Status != "" {
			out.ServiceMeta = heartbeat.Status
		}
	}
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
