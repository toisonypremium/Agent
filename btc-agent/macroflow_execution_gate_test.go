package main

import (
	"testing"
	"time"

	"btc-agent/internal/freeapi"
	"btc-agent/internal/macroflow"
)

func TestMacroExposureGateBlocksOnlyFreshExplicitRiskOff(t *testing.T) {
	now := time.Date(2026, 7, 21, 7, 0, 0, 0, time.UTC)
	freshRiskOff := freeapi.Report{GeneratedAt: now.Add(-time.Minute), MacroFlow: macroflow.Result{Regime: macroflow.RegimeRiskOff, Input: macroflow.Input{Fresh: true}}}
	if got := macroExposureBlockReason(freshRiskOff, now, 10*time.Minute); got == "" {
		t.Fatal("fresh explicit risk-off must block")
	}
	for _, report := range []freeapi.Report{
		{GeneratedAt: now.Add(-time.Minute), MacroFlow: macroflow.Result{Regime: macroflow.RegimeEarlyAltRotation, Input: macroflow.Input{Fresh: true}}},
		{GeneratedAt: now.Add(-11 * time.Minute), MacroFlow: macroflow.Result{Regime: macroflow.RegimeRiskOff, Input: macroflow.Input{Fresh: true}}},
		{GeneratedAt: now.Add(-time.Minute), MacroFlow: macroflow.Result{Regime: macroflow.RegimeUnknown, Input: macroflow.Input{Fresh: false}}},
	} {
		if got := macroExposureBlockReason(report, now, 10*time.Minute); got != "" {
			t.Fatalf("advisory/stale macro must not replace existing authority: %q", got)
		}
	}
}
