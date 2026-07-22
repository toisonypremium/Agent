package main

import (
	"strings"
	"testing"

	"btc-agent/internal/config"
)

func TestRequireAutoLiveRuntimeFailsClosed(t *testing.T) {
	base := func() config.Config {
		var cfg config.Config
		cfg.Live.LiveAutoMode = true
		cfg.Live.Enabled = true
		cfg.Live.AutoExecute = true
		cfg.Live.RequireManualConfirm = false
		cfg.Live.ProofOnly = false
		cfg.Live.SupervisorEnabled = true
		cfg.Live.OrderManagementEnabled = true
		cfg.Execution.RealTradingEnabled = true
		return cfg
	}
	cases := []struct {
		name string
		set  func(*config.Config)
		want string
	}{
		{"live auto mode disabled", func(c *config.Config) { c.Live.LiveAutoMode = false }, "live.live_auto_mode=false"},
		{"live disabled", func(c *config.Config) { c.Live.Enabled = false }, "live.enabled=false"},
		{"automatic execution disabled", func(c *config.Config) { c.Live.AutoExecute = false }, "live.auto_execute=false"},
		{"manual confirmation enabled", func(c *config.Config) { c.Live.RequireManualConfirm = true }, "live.require_manual_confirm=true"},
		{"proof only enabled", func(c *config.Config) { c.Live.ProofOnly = true }, "live.proof_only=true"},
		{"supervisor disabled", func(c *config.Config) { c.Live.SupervisorEnabled = false }, "live.supervisor_enabled=false"},
		{"order management disabled", func(c *config.Config) { c.Live.OrderManagementEnabled = false }, "live.order_management_enabled=false"},
		{"real trading disabled", func(c *config.Config) { c.Execution.RealTradingEnabled = false }, "execution.real_trading_enabled=false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BTC_AGENT_ALLOW_AUTO_LIVE", "true")
			cfg := base()
			tc.set(&cfg)
			err := requireAutoLiveRuntime(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRequireAutoLiveRuntimeRequiresExplicitEnvironmentOptIn(t *testing.T) {
	t.Setenv("BTC_AGENT_ALLOW_AUTO_LIVE", "")
	var cfg config.Config
	cfg.Live.LiveAutoMode = true
	cfg.Live.Enabled = true
	cfg.Live.AutoExecute = true
	cfg.Live.SupervisorEnabled = true
	cfg.Live.OrderManagementEnabled = true
	cfg.Execution.RealTradingEnabled = true
	if err := requireAutoLiveRuntime(cfg); err == nil || !strings.Contains(err.Error(), "BTC_AGENT_ALLOW_AUTO_LIVE=true") {
		t.Fatalf("error = %v, want explicit environment opt-in blocker", err)
	}
}
