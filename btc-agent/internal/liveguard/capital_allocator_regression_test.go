package liveguard

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"math"
	"testing"
)

func TestAllocateLiveCapitalZeroWeightFailClosed(t *testing.T) {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 1}
	cfg.Risk.MaxTotalDeploymentPerCycle = .7
	cfg.Risk.MaxSingleAssetDeployment = .7
	cfg.Live.MaxLiveNotionalTotalUSDT = 100
	cfg.Live.MaxLiveNotionalPerAssetUSDT = 100
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 10
	plan := agent2.Plan{State: agent2.StateActiveLimit, ActionPermission: agent1.Watch, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit}}}
	got := AllocateLiveCapital(cfg, plan, nil, nil)["ETHUSDT"]
	if got.BudgetUSDT != 0 || math.IsNaN(got.BudgetUSDT) || math.IsInf(got.BudgetUSDT, 0) {
		t.Fatalf("not fail closed: %+v", got)
	}
}
