package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
)

func TestBuildCapitalPlanResearchReport(t *testing.T) {
	cfg := capitalPlanTestConfig()
	s := BotRuntimeSnapshot{GeneratedAt: time.Unix(1700000000, 0), PlanState: agent2.StateWatch, BTCPermission: agent1.Watch}
	s.Plan = agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch, Assets: []agent2.AssetPlan{
		{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.95, RotationScore: 0.95, RewardRisk: 3.5, AssetFlowScore: 0.95, MMScore: 95},
		{Symbol: "SOLUSDT", State: agent2.StateWatch, SetupScore: 0.55, RotationScore: 0.55, RewardRisk: 2.0, AssetFlowScore: 0.55, MMScore: 55},
		{Symbol: "RENDERUSDT", State: agent2.StateNoTrade, SetupScore: 0.1, SetupGates: []agent2.SetupGateResult{{Name: agent2.EntryCheckData, Pass: false, Severity: agent2.SetupGateHard, Reason: "chưa đủ dữ liệu 1D"}}},
	}}
	got := buildCapitalPlanResearchReport(cfg, s)
	if len(got.Coins) != 3 {
		t.Fatalf("coins=%d", len(got.Coins))
	}
	total := 0.0
	bySymbol := map[string]CapitalPlanResearchCoin{}
	for _, coin := range got.Coins {
		total += coin.SuggestedResearchAllocation
		bySymbol[coin.Symbol] = coin
	}
	if total > 1-cfg.Portfolio.ReserveCashRatio+1e-9 {
		t.Fatalf("suggested total too high %.4f", total)
	}
	if bySymbol["RENDERUSDT"].SuggestedResearchAllocation != 0 {
		t.Fatalf("blocked coin allocation=%.4f", bySymbol["RENDERUSDT"].SuggestedResearchAllocation)
	}
	if bySymbol["ETHUSDT"].SuggestedResearchAllocation <= bySymbol["SOLUSDT"].SuggestedResearchAllocation {
		t.Fatalf("expected ETH allocation > SOL: %+v %+v", bySymbol["ETHUSDT"], bySymbol["SOLUSDT"])
	}
	md := capitalPlanResearchMarkdown(got)
	for _, want := range []string{"CAPITAL PLAN RESEARCH", "Research only", "không bypass ACTIVE_LIMIT", safetyLine} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func capitalPlanTestConfig() config.Config {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.ReserveCashRatio = 0.2
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.4, "SOLUSDT": 0.3, "RENDERUSDT": 0.1}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	return cfg
}
