package main

import (
	"context"
	"path/filepath"
	"testing"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

func TestShouldRefreshMarketDataForDoctorStaleBlockers(t *testing.T) {
	cases := []string{
		"analysis stale: age=19h max=6h",
		"plan stale: age=19h max=6h",
		"ETHUSDT 1D candle stale: age=31h max=30h",
	}
	for _, blocker := range cases {
		doctor := liveguard.RuntimeDoctorResult{Status: liveguard.DoctorBlock, Blockers: []string{blocker}}
		if !shouldRefreshMarketDataForDoctor(doctor) {
			t.Fatalf("expected refresh for blocker %q", blocker)
		}
	}
}

func TestShouldRefreshMarketDataForDoctorDataHealthBlockers(t *testing.T) {
	doctor := liveguard.RuntimeDoctorResult{
		Status: liveguard.DoctorBlock,
		DataHealth: liveguard.DataHealthResult{
			Status:   liveguard.DataHealthBlock,
			Blockers: []string{"analysis stale: age=7h max=6h"},
		},
	}
	if !shouldRefreshMarketDataForDoctor(doctor) {
		t.Fatal("expected refresh for data health stale blocker")
	}
}

func TestShouldRefreshMarketDataForDoctorIgnoresNonStaleBlocks(t *testing.T) {
	cases := []liveguard.RuntimeDoctorResult{
		{Status: liveguard.DoctorOK, Blockers: []string{"analysis stale: age=7h max=6h"}},
		{Status: liveguard.DoctorBlock, Blockers: []string{"operator halt active"}},
		{Status: liveguard.DoctorBlock, Blockers: []string{"account check not pass"}},
	}
	for _, doctor := range cases {
		if shouldRefreshMarketDataForDoctor(doctor) {
			t.Fatalf("unexpected refresh for %+v", doctor)
		}
	}
}

func TestUpdateDoctorBlockWatchdogAutoHaltsAtThreshold(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	cfg := validTestConfigForScheduler()
	cfg.Live.AutoHaltAfterErrors = 2
	doctor := liveguard.RuntimeDoctorResult{Status: liveguard.DoctorBlock, Summary: "blocked"}
	count := updateDoctorBlockWatchdog(context.Background(), cfg, db, doctor, 0)
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
	halted, _ := db.IsHalted()
	if halted {
		t.Fatal("halted before threshold")
	}
	count = updateDoctorBlockWatchdog(context.Background(), cfg, db, doctor, count)
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
	halted, _ = db.IsHalted()
	if !halted {
		t.Fatal("expected operator halt after threshold")
	}
}

func TestUpdateDoctorBlockWatchdogResetsOnOK(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := validTestConfigForScheduler()
	count := updateDoctorBlockWatchdog(context.Background(), cfg, db, liveguard.RuntimeDoctorResult{Status: liveguard.DoctorOK}, 2)
	if count != 0 {
		t.Fatalf("count=%d want 0", count)
	}
}

func validTestConfigForScheduler() config.Config {
	var cfg config.Config
	cfg.Notify.Enabled = false
	cfg.Notify.Provider = "telegram"
	cfg.Live.AutoHaltAfterErrors = 3
	return cfg
}
