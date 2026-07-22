package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

type dashboardResponse struct {
	GeneratedAt string `json:"generated_at"`
	Mode        string `json:"mode"`
	Safety      struct {
		Halted      bool   `json:"halted"`
		Authority   string `json:"authority"`
		OrderPolicy string `json:"order_policy"`
		LiveEnabled bool   `json:"live_enabled"`
	} `json:"safety"`
	Market    any               `json:"market,omitempty"`
	Plan      any               `json:"plan,omitempty"`
	Positions any               `json:"positions"`
	Orders    any               `json:"orders"`
	Events    any               `json:"events"`
	Errors    map[string]string `json:"errors,omitempty"`
}

// runWeb starts a read-only local dashboard. It deliberately has no mutation routes.
func runWeb(cfg config.Config, db *storage.DB, args []string) error {
	listen := argValue(args, "--listen")
	if listen == "" {
		listen = "127.0.0.1:20129"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeDashboardJSON(w, buildDashboardResponse(cfg, db))
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeDashboardJSON(w, map[string]string{"status": "ok"})
	})
	mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join("web", "index.html"))
	})
	server := &http.Server{Addr: listen, Handler: dashboardSecurityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	return server.ListenAndServe()
}

func buildDashboardResponse(cfg config.Config, db *storage.DB) dashboardResponse {
	out := dashboardResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Mode: cfg.App.Mode, Errors: map[string]string{}}
	out.Safety.OrderPolicy = "READ_ONLY — no order controls exposed"
	out.Safety.LiveEnabled = cfg.Live.Enabled
	halted, err := db.IsHalted()
	if err != nil {
		out.Errors["safety"] = "unable to verify operator halt state"
		halted = true
	}
	out.Safety.Halted = halted
	if halted {
		out.Safety.Authority = "HALTED"
	} else {
		out.Safety.Authority = "BLOCKED"
	}
	if market, err := db.LatestAnalysis(); err == nil {
		out.Market = market
	} else {
		out.Errors["market"] = "no market analysis is available"
	}
	if plan, err := db.LatestPlan(); err == nil {
		out.Plan = plan
	} else {
		out.Errors["plan"] = "no accumulation plan is available"
	}
	if positions, err := db.LivePositions(); err == nil {
		out.Positions = positions
	} else {
		out.Errors["positions"] = "unable to read the position ledger"
	}
	if orders, err := db.OpenLiveOrdersDetailed(); err == nil {
		out.Orders = orders
	} else {
		out.Errors["orders"] = "unable to read open orders"
	}
	if events, err := db.PendingRuntimeEvents(12); err == nil {
		out.Events = events
	} else {
		out.Errors["events"] = "unable to read runtime events"
	}
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out
}

func writeDashboardJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}

func dashboardSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
