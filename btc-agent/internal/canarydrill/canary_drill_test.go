package canarydrill

import (
	"strings"
	"testing"

	"btc-agent/internal/config"
)

func defaultTestConfig() config.Config {
	var cfg config.Config
	cfg.Live.Enabled = false
	cfg.Live.ProofOnly = true
	cfg.Live.CanaryMode = false
	cfg.Live.CanaryMaxNotionalUSDT = 0
	cfg.Execution.RealTradingEnabled = false
	return cfg
}

func TestRunPassesWithDefaultConfig(t *testing.T) {
	cfg := defaultTestConfig()
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected passed=true, got summary: %s\nchecks: %+v", report.Summary, report.Checks)
	}
	if report.Safety.SimulationOnly != true {
		t.Error("simulation_only must be true")
	}
	if report.Safety.RealExchangeCalls != 0 {
		t.Errorf("real_exchange_calls must be 0, got %d", report.Safety.RealExchangeCalls)
	}
}

func TestRunMultipleCyclesStable(t *testing.T) {
	cfg := defaultTestConfig()
	for _, cycles := range []int{1, 5, 20} {
		report, err := Run(cfg, cycles)
		if err != nil {
			t.Fatalf("cycles=%d Run failed: %v", cycles, err)
		}
		if !report.Passed {
			t.Fatalf("cycles=%d expected passed=true, summary: %s", cycles, report.Summary)
		}
		if report.Cycles != cycles {
			t.Errorf("cycles=%d report.Cycles=%d mismatch", cycles, report.Cycles)
		}
	}
}

func TestRunMarkdownContainsNoRealOrder(t *testing.T) {
	cfg := defaultTestConfig()
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	md := RenderMarkdown(report)
	if !strings.Contains(md, "No real order was placed.") {
		t.Errorf("markdown missing required phrase 'No real order was placed.'\ngot:\n%s", md)
	}
	if !strings.Contains(md, "simulation_only: true") {
		t.Errorf("markdown missing 'simulation_only: true'")
	}
	if !strings.Contains(md, "real_exchange_calls: 0") {
		t.Errorf("markdown missing 'real_exchange_calls: 0'")
	}
}

func TestRunRealExchangeCallsAlwaysZero(t *testing.T) {
	cfg := defaultTestConfig()
	for i := 0; i < 5; i++ {
		report, err := Run(cfg, 3)
		if err != nil {
			t.Fatal(err)
		}
		if report.Safety.RealExchangeCalls != 0 {
			t.Errorf("iteration %d: real_exchange_calls=%d, expected 0", i, report.Safety.RealExchangeCalls)
		}
	}
}

func TestFilterRejectionsDetected(t *testing.T) {
	cfg := defaultTestConfig()
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range report.Checks {
		if c.Name == "filter_rejections" {
			found = true
			if !c.Passed {
				t.Errorf("filter_rejections check failed: %s", c.Detail)
			}
		}
	}
	if !found {
		t.Error("filter_rejections check not present in report")
	}
}

func TestAuthorityGuardCheck(t *testing.T) {
	cfg := defaultTestConfig()
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Checks {
		if c.Name == "authority_guard" {
			if !c.Passed {
				t.Errorf("authority_guard check failed: %s", c.Detail)
			}
			return
		}
	}
	t.Error("authority_guard check not present in report")
}

func TestCanaryCapInformationalWhenLiveDisabled(t *testing.T) {
	cfg := defaultTestConfig()
	// cap not set
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Checks {
		if c.Name == "canary_cap" && !c.Passed {
			t.Errorf("canary_cap should pass when live disabled, got: %s", c.Detail)
		}
	}
}

func TestCanaryCapEnforcedWhenSet(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Live.CanaryMaxNotionalUSDT = 1.00 // drill notional is 1.39, should still pass since drill is informational
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	// With cap=1.00 < 1.39, canary_cap check should FAIL
	for _, c := range report.Checks {
		if c.Name == "canary_cap" {
			if c.Passed {
				t.Errorf("expected canary_cap to fail when cap=1.00 < drill notional 1.39, got: %s", c.Detail)
			}
			return
		}
	}
	t.Error("canary_cap check not present")
}

func TestRunDefaultCyclesWhenZero(t *testing.T) {
	cfg := defaultTestConfig()
	report, err := Run(cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cycles != 20 {
		t.Errorf("expected default 20 cycles, got %d", report.Cycles)
	}
}

func TestSafetySnapshotReflectsConfig(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.CanaryMode = true
	report, err := Run(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safety.LiveEnabled {
		t.Error("safety.live_enabled should be true when cfg.Live.Enabled=true")
	}
	if !report.Safety.CanaryMode {
		t.Error("safety.canary_mode should be true when cfg.Live.CanaryMode=true")
	}
	// Still simulation-only regardless of config flags
	if !report.Safety.SimulationOnly {
		t.Error("simulation_only must always be true in drill")
	}
}
