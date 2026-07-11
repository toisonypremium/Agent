package main

import (
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/microstructure"
)

func TestApplyMicrostructurePermissionGateCapsAllowed(t *testing.T) {
	cfg := config.Config{}
	cfg.Microstructure.Enabled = true
	cfg.Microstructure.RequireFreshForActive = true
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Allowed, PermissionReason: "base allowed", BTCPrice: 100, MarketRegime: "RANGE"}
	summary := microstructure.Summary{Enabled: true, Status: microstructure.StatusBlock, Blockers: []string{"BTC microstructure stale/missing"}}
	out := applyMicrostructurePermissionGate(cfg, analysis, summary)
	if out.ActionPermission != agent1.Watch {
		t.Fatalf("expected WATCH cap, got %s", out.ActionPermission)
	}
	if out.Microstructure.Status != microstructure.StatusBlock {
		t.Fatalf("expected summary attached: %+v", out.Microstructure)
	}
}

func TestApplyMicrostructureAssetGateDowngradesActive(t *testing.T) {
	cfg := config.Config{}
	cfg.Microstructure.Enabled = true
	cfg.Microstructure.RequireFreshForActive = true
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1}}, Reason: "active"}}}
	summary := microstructure.BuildSummary(true, "BTCUSDT", []microstructure.Snapshot{{Symbol: "BTCUSDT", Timestamp: time.Now(), Health: microstructure.Health{Fresh: true}}, {Symbol: "ETHUSDT", Timestamp: time.Now(), Health: microstructure.Health{Fresh: false, Blockers: []string{"ETH stale"}}}}, 1, time.Now())
	out := applyMicrostructureAssetGate(cfg, plan, summary)
	if out.State == agent2.StateActiveLimit {
		t.Fatal("plan should not remain ACTIVE_LIMIT when asset microstructure is stale")
	}
	if out.Assets[0].State != agent2.StateWatch || len(out.Assets[0].Layers) != 0 {
		t.Fatalf("asset not downgraded safely: %+v", out.Assets[0])
	}
}
