package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type cloudDashboardClient struct {
	base, key string
	client    *http.Client
}

func newCloudDashboardClient() *cloudDashboardClient {
	return &cloudDashboardClient{base: strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/"), key: strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY")), client: &http.Client{Timeout: 8 * time.Second}}
}
func (c *cloudDashboardClient) enabled() bool { return c.base != "" && c.key != "" }
func (c *cloudDashboardClient) selectRows(ctx context.Context, view, order, limit string) (any, error) {
	if !c.enabled() {
		return []any{}, fmt.Errorf("cloud dashboard not configured")
	}
	q := url.Values{"select": {"*"}}
	if order != "" {
		q.Set("order", order)
	}
	if limit != "" {
		q.Set("limit", limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/rest/v1/"+view+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Supabase read status=%d", resp.StatusCode)
	}
	var out any
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
func registerCloudDashboardAPI(mux *http.ServeMux) {
	c := newCloudDashboardClient()
	routes := map[string]struct{ view, order, limit string }{
		"/api/cloud/v1/summary":   {"dashboard_operational_summary", "", ""},
		"/api/cloud/v1/alerts":    {"dashboard_recent_alerts", "created_at.desc", "200"},
		"/api/cloud/v1/positions": {"dashboard_positions", "reconciled_at.desc", "200"},
		"/api/cloud/v1/decisions": {"dashboard_recent_decisions", "decided_at.desc", "200"},
		"/api/cloud/v1/orders":    {"dashboard_recent_orders", "created_at.desc", "200"},
		"/api/cloud/v1/artifacts": {"dashboard_artifacts", "created_at.desc", "200"},
		"/api/cloud/v1/llm-usage": {"dashboard_llm_usage_daily", "usage_day.desc", "200"},
	}
	for path, spec := range routes {
		path, spec := path, spec
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			rows, err := c.selectRows(r.Context(), spec.view, spec.order, spec.limit)
			if err != nil {
				writeWebJSON(w, map[string]any{"status": "degraded", "source": "supabase", "error": err.Error(), "data": []any{}})
				return
			}
			writeWebJSON(w, map[string]any{"status": "fresh", "source": "supabase", "generated_at": time.Now().UTC(), "data": rows})
		})
	}
	mux.HandleFunc("/api/cloud/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		summary, err := c.selectRows(r.Context(), "dashboard_operational_summary", "", "")
		status := "fresh"
		msg := "Supabase connected"
		if err != nil {
			status = "degraded"
			msg = err.Error()
		}
		writeWebJSON(w, map[string]any{"status": status, "supabase": c.enabled(), "r2": strings.TrimSpace(os.Getenv("R2_BUCKET")) != "" || strings.TrimSpace(os.Getenv("R2_PRESIGNED_PUT_URL")) != "", "message": msg, "summary": summary, "generated_at": time.Now().UTC()})
	})
}
