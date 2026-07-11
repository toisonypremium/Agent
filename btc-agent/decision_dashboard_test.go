package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

func TestBuildDecisionDashboardReadiness(t *testing.T) {
	snapshot := BotRuntimeSnapshot{GeneratedAt: time.Unix(1700000000, 0), Mode: "live-auto", AutoLiveAllowed: true, LiveEnabled: true, AutoExecute: true, RealTradingEnabled: true, PlanState: agent2.StateWatch, BTCPermission: agent1.Watch, Safety: safetyLine}
	scenario := ScenarioReport{GeneratedAt: snapshot.GeneratedAt, PlanState: snapshot.PlanState, BTCPermission: snapshot.BTCPermission, Blockers: []string{"plan chưa ACTIVE_LIMIT"}}
	technical := TechnicalScorecardReport{Summary: "tech", Coins: []TechnicalScorecardCoin{{Symbol: "ETHUSDT", TechnicalScore: 0.7}}}
	capital := CapitalPlanResearchReport{Summary: "capital"}
	filter := FilterAttributionReport{Summary: "filters"}
	dash := buildDecisionDashboard(snapshot, scenario, technical, capital, filter, agent2.UniverseResearchReport{})
	if !dash.BotReady {
		t.Fatal("expected bot ready runtime")
	}
	if dash.MarketReady || dash.CanSubmitNow {
		t.Fatalf("expected market not ready/can submit false: %+v", dash)
	}
	if dash.BestProductionCoin != "ETHUSDT" {
		t.Fatalf("best production=%s", dash.BestProductionCoin)
	}
}

func TestBuildDecisionDashboardMarketReady(t *testing.T) {
	snapshot := BotRuntimeSnapshot{GeneratedAt: time.Unix(1700000000, 0), Mode: "live-auto", AutoLiveAllowed: true, LiveEnabled: true, AutoExecute: true, RealTradingEnabled: true, PlanState: agent2.StateActiveLimit, BTCPermission: agent1.Allowed, CanSubmitLiveOrder: true}
	dash := buildDecisionDashboard(snapshot, ScenarioReport{}, TechnicalScorecardReport{}, CapitalPlanResearchReport{}, FilterAttributionReport{}, agent2.UniverseResearchReport{TopCandidates: []agent2.UniverseResearchRow{{Symbol: "LINKUSDT", DataStatus: agent2.UniverseDataOK, OpportunityVerdict: agent2.OpportunityVerdictNormal}}})
	if !dash.MarketReady || !dash.CanSubmitNow {
		t.Fatalf("expected ready dashboard: %+v", dash)
	}
	if dash.BestUniverseCoin != "LINKUSDT" {
		t.Fatalf("best universe=%s", dash.BestUniverseCoin)
	}
	md := decisionDashboardMarkdown(dash)
	for _, want := range []string{"DECISION DASHBOARD", "Safety", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}
