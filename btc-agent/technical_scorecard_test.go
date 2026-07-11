package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

func TestBuildTechnicalScorecardReport(t *testing.T) {
	s := BotRuntimeSnapshot{GeneratedAt: time.Unix(1700000000, 0), PlanState: agent2.StateWatch, BTCPermission: agent1.Watch, Safety: safetyLine}
	s.Plan = agent2.Plan{State: agent2.StateWatch, ActionPermission: agent1.Watch, Assets: []agent2.AssetPlan{
		{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.82, RotationScore: 0.8, RewardRisk: 2.2, SetupGates: []agent2.SetupGateResult{{Name: agent2.EntryCheckDiscountZone, Pass: false, Severity: agent2.SetupGateSoft, Score: 0.6, Reason: "giá chưa vào discount zone"}}, SoftBlockers: []string{"giá chưa vào discount zone"}, NextTrigger: "Chờ giá về support"},
		{Symbol: "SOLUSDT", State: agent2.StateNoTrade, SetupScore: 0.1, SetupGates: []agent2.SetupGateResult{{Name: agent2.EntryCheckData, Pass: false, Severity: agent2.SetupGateHard, Reason: "chưa đủ dữ liệu 1D"}}, HardBlockers: []string{"chưa đủ dữ liệu 1D"}},
	}}
	got := buildTechnicalScorecardReport(s)
	if len(got.Coins) != 2 {
		t.Fatalf("coins=%d", len(got.Coins))
	}
	if got.Coins[0].TopBlockerKey != agent2.EntryCheckDiscountZone {
		t.Fatalf("top blocker=%s", got.Coins[0].TopBlockerKey)
	}
	if got.Coins[1].Verdict != TechnicalVerdictBlockData {
		t.Fatalf("expected BLOCK_DATA, got %s", got.Coins[1].Verdict)
	}
	md := technicalScorecardMarkdown(got)
	for _, want := range []string{"TECHNICAL SCORECARD", "Research only", "không bypass ACTIVE_LIMIT", safetyLine} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestTechnicalVerdictEntryReady(t *testing.T) {
	row := TechnicalScorecardCoin{State: agent2.StateActiveLimit, TechnicalScore: 0.95}
	if got := technicalVerdict(row); got != TechnicalVerdictEntryReady {
		t.Fatalf("verdict=%s", got)
	}
}
