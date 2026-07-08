package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/market"
)

func TestConservativeOHLCVExitDecisionInvalidationWinsAmbiguity(t *testing.T) {
	candle := market.Candle{High: 112, Low: 88, Close: 100}
	got := ConservativeOHLCVExitDecision(candle, 100, 90, 110)
	if got.Exit != OHLCVExitInvalidation || !got.Ambiguous || !got.Conservative {
		t.Fatalf("expected conservative invalidation ambiguity, got %+v", got)
	}
	if !strings.Contains(got.Reason, "same-candle") {
		t.Fatalf("expected ambiguity reason, got %q", got.Reason)
	}
}

func TestConservativeOHLCVExitDecisionTakeProfitOnly(t *testing.T) {
	candle := market.Candle{High: 112, Low: 95, Close: 111}
	got := ConservativeOHLCVExitDecision(candle, 100, 90, 110)
	if got.Exit != OHLCVExitTakeProfit || got.Ambiguous {
		t.Fatalf("expected take profit, got %+v", got)
	}
}

func TestAgent2SimulationInvalidationBeatsSameCandleTakeProfit(t *testing.T) {
	sim := Agent2Simulation{Enabled: true, Assets: map[string]AssetSimStats{"ETHUSDT": {Symbol: "ETHUSDT"}}, Diagnostics: newAgent2Diagnostics([]string{"ETHUSDT"})}
	positions := map[string]*simPosition{"ETHUSDT": {qty: 1, cost: 100, invalidation: 90, firstFillIndex: 0}}
	orders := map[string][]simOrder{"ETHUSDT": {}}
	assets := map[string][]market.Candle{"ETHUSDT": {
		{High: 100, Low: 100, Close: 100},
		{High: 112, Low: 88, Close: 105},
	}}
	processOrdersAndPositionsWithOverrides(&sim, orders, positions, assets, 1, 3, "2026-01-02", SimulationOverrides{TakeProfitPct: 0.10})
	stats := sim.Assets["ETHUSDT"]
	if stats.Invalidations != 1 || stats.TakeProfits != 0 {
		t.Fatalf("expected invalidation wins, stats=%+v events=%+v", stats, sim.Diagnostics.Events)
	}
	if len(sim.Diagnostics.Events) == 0 || !strings.Contains(sim.Diagnostics.Events[0].Reason, "same-candle") {
		t.Fatalf("missing ambiguity event reason: %+v", sim.Diagnostics.Events)
	}
}

func TestAgent2DiagnosticsMentionOHLCVAmbiguity(t *testing.T) {
	d := newAgent2Diagnostics([]string{"ETHUSDT"})
	found := false
	for _, note := range d.Notes {
		if strings.Contains(note, "Conservative OHLCV ambiguity") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing OHLCV ambiguity note: %+v", d.Notes)
	}
}
