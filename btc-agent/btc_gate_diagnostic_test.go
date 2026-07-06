package main

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestBTCGateDiagnosticMarkdownShowsUnlockRoutes(t *testing.T) {
	report := buildBTCGateDiagnosticReport(btcGateDiagnosticTestAnalysis())
	md := btcGateDiagnosticMarkdown(report)
	for _, want := range []string{"BTC GATE DIAGNOSTIC", "Current: WATCH", "Gap to ARMED", "Gap to ALLOWED", "Frame contribution", "Flow route", "TREND_TO_ARMED", "FLOW_PROMOTE_ARMED", "HARD_BLOCKERS: pass current=none", "No order was placed"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestBTCGateFrameContributionMath(t *testing.T) {
	got := btcGateFrameContributions(btcGateDiagnosticTestAnalysis())
	if len(got) != 3 {
		t.Fatalf("expected 3 frame contributions: %+v", got)
	}
	want := map[string]float64{"1w": 0, "1d": 16, "4h": 9.75}
	for _, frame := range got {
		if frame.Contribution != want[frame.Timeframe] {
			t.Fatalf("%s contribution=%v want %v", frame.Timeframe, frame.Contribution, want[frame.Timeframe])
		}
	}
}

func btcGateDiagnosticTestAnalysis() agent1.MarketAnalysis {
	return agent1.MarketAnalysis{
		BTCPrice:           63702,
		ActionPermission:   agent1.Watch,
		PermissionReason:   "trend 25.8 chưa đủ ARMED",
		MarketRegime:       "RANGE",
		RiskLevel:          agent1.Medium,
		FallingKnifeRisk:   agent1.Low,
		FomoRisk:           agent1.Low,
		TrendScore:         25.75,
		PrimarySupportZone: market.Zone{Name: "support", Low: 57800.19, High: 59774.5204},
		ResistanceZone:     market.Zone{Name: "resistance", Low: 80504.9896, High: 82479.32},
		Frames: map[string]market.FrameSignal{
			"1w": {Bias: "BEARISH", TrendScore: 0, EMA20: 71166.36, EMA50: 80138.88, EMA200: 68641.24, RSI14: 38.3, Structure: market.Structure{Label: "LL", LowerLowCount: 2, HigherHighCount: 2}},
			"1d": {Bias: "RANGE", TrendScore: 40, EMA20: 62535.06, EMA50: 65769.94, EMA200: 75638.70, RSI14: 51.9, Structure: market.Structure{Label: "LL", LowerLowCount: 2, HigherHighCount: 1}},
			"4h": {Bias: "RANGE", TrendScore: 65, EMA20: 62729.53, EMA50: 61946.11, EMA200: 63992.93, RSI14: 62.8, Structure: market.Structure{Label: "HH", LowerLowCount: 1, HigherHighCount: 4}},
		},
		Flow: flow.MultiFrame{
			Bias:  flow.BiasNeutral,
			Score: 0,
			Daily: flow.Signal{Diagnostics: flow.FlowDiagnostics{NextBullTrigger: "chờ sweep low dưới support"}},
		},
	}
}
