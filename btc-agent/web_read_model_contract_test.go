package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/storage"
	"path/filepath"
	"testing"
)

func TestDashboardV31IncludesEveryRequiredDomain(t *testing.T) {
	db, e := storage.Open(filepath.Join(t.TempDir(), "v31.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	m := buildDashboardV3(config.Config{}, db)
	if m.SchemaVersion != "dashboard-v3.1" {
		t.Fatal(m.SchemaVersion)
	}
	for _, n := range []string{"tong_quan", "thi_truong", "danh_muc", "hermes", "rui_ro", "circuit", "van_hanh", "lich_trinh", "nhat_ky"} {
		if _, ok := m.Domains[n]; !ok {
			t.Fatal("missing " + n)
		}
	}
}
func TestEventSummaryUsesWhitelistedSummary(t *testing.T) {
	if g := eventSummary(`{"summary":"chu kỳ hoàn tất","secret":"do-not-show"}`); g != "chu kỳ hoàn tất" {
		t.Fatal(g)
	}
	if g := eventSummary(`{"secret":"do-not-show"}`); g == "do-not-show" {
		t.Fatal("secret exposed")
	}
}
