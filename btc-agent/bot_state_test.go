package main

import (
	"testing"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
)

func TestCanSubmitLiveOrderFromSnapshotMatrix(t *testing.T) {
	base := BotRuntimeSnapshot{
		Mode:                 "live-auto",
		DryRun:               false,
		AutoLiveAllowed:      true,
		LiveEnabled:          true,
		AutoExecute:          true,
		RequireManualConfirm: false,
		ProofOnly:            false,
		RealTradingEnabled:   true,
		OperatorHalt:         false,
		DoctorStatus:         string(liveguard.DoctorOK),
		PlanState:            agent2.StateActiveLimit,
		DesiredOrders:        1,
		BTC:                  BotBTCSnapshot{AccumulationPhase: string(accumulation.PhaseConfirmed)},
	}

	tests := []struct {
		name string
		edit func(*BotRuntimeSnapshot)
		want bool
	}{
		{name: "active limit with desired and gates pass", want: true},
		{name: "scout with no desired", edit: func(s *BotRuntimeSnapshot) { s.PlanState = agent2.StateScout; s.DesiredOrders = 0 }, want: false},
		{name: "armed with desired", edit: func(s *BotRuntimeSnapshot) { s.PlanState = agent2.StateArmed }, want: false},
		{name: "active limit without desired", edit: func(s *BotRuntimeSnapshot) { s.DesiredOrders = 0 }, want: false},
		{name: "dry run blocks", edit: func(s *BotRuntimeSnapshot) { s.DryRun = true }, want: false},
		{name: "doctor block blocks", edit: func(s *BotRuntimeSnapshot) { s.DoctorStatus = string(liveguard.DoctorBlock) }, want: false},
		{name: "operator halt blocks", edit: func(s *BotRuntimeSnapshot) { s.OperatorHalt = true }, want: false},
		{name: "auto live env blocks", edit: func(s *BotRuntimeSnapshot) { s.AutoLiveAllowed = false }, want: false},
		{name: "manual confirm blocks", edit: func(s *BotRuntimeSnapshot) { s.RequireManualConfirm = true }, want: false},
		{name: "proof only blocks", edit: func(s *BotRuntimeSnapshot) { s.ProofOnly = true }, want: false},
		{name: "real trading disabled blocks", edit: func(s *BotRuntimeSnapshot) { s.RealTradingEnabled = false }, want: false},
		{name: "btc accumulation not confirmed blocks", edit: func(s *BotRuntimeSnapshot) { s.BTC.AccumulationPhase = string(accumulation.PhaseReclaim) }, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			if tt.edit != nil {
				tt.edit(&s)
			}
			if got := canSubmitLiveOrderFromSnapshot(s); got != tt.want {
				t.Fatalf("canSubmitLiveOrderFromSnapshot()=%v want %v snapshot=%+v", got, tt.want, s)
			}
		})
	}
}
