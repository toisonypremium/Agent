package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudDashboardSelectRows(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "key" {
			t.Error("missing key")
		}
		if r.URL.Path != "/rest/v1/dashboard_recent_alerts" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"severity":"INFO"}]`))
	}))
	defer s.Close()
	c := &cloudDashboardClient{base: s.URL, key: "key", client: s.Client()}
	v, err := c.selectRows(context.Background(), "dashboard_recent_alerts", "created_at.desc", "200")
	if err != nil || v == nil {
		t.Fatalf("v=%v err=%v", v, err)
	}
}
func TestCloudDashboardDisabled(t *testing.T) {
	c := &cloudDashboardClient{}
	if _, err := c.selectRows(context.Background(), "x", "", ""); err == nil {
		t.Fatal("expected error")
	}
}
