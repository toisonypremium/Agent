package main

import (
	"testing"

	"btc-agent/internal/opsplan"
)

func TestLiveAutoNearUnlockEventMatrix(t *testing.T) {
	tests := []struct {
		name   string
		report opsplan.Report
		want   string
	}{
		{name: "one near signal no event", report: opsplan.Report{Market: opsplan.MarketPlan{PlanState: "ARMED", Permission: "WATCH", AccumulationPhase: "MARKDOWN"}}, want: ""},
		{name: "two near signals", report: opsplan.Report{Market: opsplan.MarketPlan{PlanState: "ARMED", Permission: "ARMED", AccumulationPhase: "MARKDOWN"}}, want: "LIVE_AUTO_NEAR_UNLOCK"},
		{name: "all market gates ready requires dry run", report: opsplan.Report{Market: opsplan.MarketPlan{PlanState: "ACTIVE_LIMIT", Permission: "ALLOWED", AccumulationPhase: "ACCUMULATION_CONFIRMED"}, Capital: opsplan.CapitalPlan{ExecutableNowUSDT: 10}}, want: "LIVE_AUTO_READY_DRY_RUN_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := liveAutoNearUnlockEvent(tt.report)
			if got != tt.want {
				t.Fatalf("event=%q want %q", got, tt.want)
			}
		})
	}
}
