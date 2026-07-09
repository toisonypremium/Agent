package main

import (
	"testing"

	"btc-agent/internal/liveguard"
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
