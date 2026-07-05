package liveguard

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

func TestEvaluateRiskGovernorBlocksDataHealthAndPanic(t *testing.T) {
	cfg := riskGovConfig()
	analysis := agent1.MarketAnalysis{MarketRegime: "PANIC_SELLING", FallingKnifeRisk: agent1.High, FomoRisk: agent1.Low}
	got := EvaluateRiskGovernor(cfg, analysis, agent2.Plan{}, nil, nil, DataHealthResult{Status: DataHealthBlock, Blockers: []string{"stale data"}}, ReconcileSafetyResult{Status: ReconcileClean})
	if got.Status != RiskGovernorBlock {
		t.Fatalf("status=%s summary=%s", got.Status, got.Summary)
	}
	joined := strings.Join(got.Blockers, " ")
	if !strings.Contains(joined, "data health block") || !strings.Contains(joined, "PANIC_SELLING") || !strings.Contains(joined, "falling knife") {
		t.Fatalf("missing blockers: %+v", got)
	}
}

func TestEvaluateRiskGovernorBlocksExposureAboveCap(t *testing.T) {
	cfg := riskGovConfig()
	analysis := agent1.MarketAnalysis{MarketRegime: "RANGE", FallingKnifeRisk: agent1.Low, FomoRisk: agent1.Low}
	open := []live.OrderStatus{{Symbol: "ETHUSDT", Price: 100, Quantity: 0.05, Notional: 5}}
	positions := []live.LivePosition{{Symbol: "ETHUSDT", Quantity: 0.1, AvgEntryPrice: 100, CostBasis: 20}}
	got := EvaluateRiskGovernor(cfg, analysis, agent2.Plan{}, open, positions, DataHealthResult{Status: DataHealthOK}, ReconcileSafetyResult{Status: ReconcileClean})
	if got.Status != RiskGovernorBlock || !strings.Contains(strings.Join(got.Blockers, " "), "exceeds total cap") {
		t.Fatalf("expected exposure cap block: %+v", got)
	}
}

func TestEvaluateRiskGovernorWarnsOnWatchPermission(t *testing.T) {
	cfg := riskGovConfig()
	analysis := agent1.MarketAnalysis{MarketRegime: "DOWNTREND", ActionPermission: agent1.Watch, FallingKnifeRisk: agent1.Low, FomoRisk: agent1.Low}
	got := EvaluateRiskGovernor(cfg, analysis, agent2.Plan{State: agent2.StateWatch}, nil, nil, DataHealthResult{Status: DataHealthOK}, ReconcileSafetyResult{Status: ReconcileClean})
	if got.Status != RiskGovernorWarn || len(got.Blockers) != 0 || len(got.Warnings) == 0 {
		t.Fatalf("expected warning only: %+v", got)
	}
}

func riskGovConfig() config.Config {
	var cfg config.Config
	cfg.Live.MaxLiveNotionalTotalUSDT = 10
	cfg.Live.MaxOpenLiveOrdersTotal = 3
	return cfg
}
