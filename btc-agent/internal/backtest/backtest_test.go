package backtest

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func btcCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64(i%20)
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 1, Volume: 1000}
	}
	return out
}

func TestRunBTCRequiresEnoughCandles(t *testing.T) {
	_, err := RunBTC(Config{MinWindow1D: 30, HorizonDays: []int{1, 3, 7}}, btcCandles(35))
	if err == nil || !strings.Contains(err.Error(), "not enough BTC 1d candles") {
		t.Fatalf("expected not enough data error, got %v", err)
	}
}

func TestRunBTCCountsFlowBiases(t *testing.T) {
	c := btcCandles(90)
	for _, idx := range []int{40, 60} {
		c[idx].Open = 118
		c[idx].High = 135
		c[idx].Low = 112
		c[idx].Close = 116
		c[idx].Volume = 2400
	}
	got, err := RunBTC(Config{MinWindow1D: 30, HorizonDays: []int{1, 3}}, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowsTested == 0 {
		t.Fatalf("expected tested windows: %+v", got)
	}
	if got.FlowCounts[flow.BiasBullTrap]+got.FlowCounts[flow.BiasDistribution] == 0 {
		t.Fatalf("expected bearish flow counts: %+v", got.FlowCounts)
	}
	if got.SignalDensity <= 0 {
		t.Fatalf("expected positive signal density: %+v", got.SignalDensity)
	}
	if got.FlowParams.VolumeHighMultiplier <= 0 {
		t.Fatalf("expected flow params in result: %+v", got.FlowParams)
	}
	if got.ByBias[flow.BiasNeutral].Count == 0 {
		t.Fatalf("expected neutral stats: %+v", got.ByBias)
	}
}

func TestWorstDrawdown(t *testing.T) {
	candles := []market.Candle{{Low: 95}, {Low: 90}, {Low: 98}}
	got := worstDrawdown(candles, 100)
	if got != -0.10 {
		t.Fatalf("drawdown=%v want -0.10", got)
	}
}

func TestMarkdownContainsAuditSections(t *testing.T) {
	got, err := RunBTC(Config{MinWindow1D: 30, HorizonDays: []int{1, 3}}, btcCandles(70))
	if err != nil {
		t.Fatal(err)
	}
	got.Agent2Simulation = Agent2Simulation{Enabled: true, Assets: map[string]AssetSimStats{"ETHUSDT": {Symbol: "ETHUSDT", PlansCreated: 1, OrdersPlaced: 3, OrdersFilled: 1}}, Summary: "Agent 2 simulation test", Diagnostics: Agent2Diagnostics{WindowsTested: 2, Agent1PermissionCount: map[agent1.Permission]int{agent1.Allowed: 1, agent1.Watch: 1}, Agent1RegimeCounts: map[string]int{"RANGE": 1}, Agent1RiskCounts: map[string]int{"risk=LOW falling=LOW fomo=LOW": 1}, AssetReasonCounts: map[string]map[string]int{"ETHUSDT": {"giá chưa vào discount zone": 1}}}}
	got.LayerAudit = LayerAuditResult{Enabled: true, Summary: "layer audit test", Rows: []LayerAuditRow{{Symbol: "ETHUSDT", InvalidationBuffer: 0.015, LayerDepthMultiplier: 1, PlansCreated: 1, OrdersFilled: 1, Verdict: "WATCH"}}}
	got.ExitAudit = ExitAuditResult{Enabled: true, Summary: "exit audit test", Rows: []ExitAuditRow{{Symbol: "ETHUSDT", TakeProfitPct: 0.03, TimeStopDays: 3, PlansCreated: 1, OrdersFilled: 1, TakeProfits: 1, Verdict: "WATCH"}}}
	got.WalkForwardVerdict = WalkForwardVerdict{Status: "INSUFFICIENT_DATA", EvaluationSamples: 10, Reason: "low sample"}
	got.AccumulationWalkForward = AccumulationWalkForwardReport{Status: "INSUFFICIENT_DATA", Summary: "low accumulation sample"}
	md := Markdown(got)
	for _, want := range []string{"BTC FLOW BACKTEST / AUDIT", "Detector params", "Flow bias counts", "Forward returns", "Drawdown audit", "Agent 2 Layer Simulation", "Diagnostics", "Agent 1 permissions", "Top asset block reasons", "Agent 2 Asset Flow Entry Forward Audit", "Agent 2 Near-Miss Forced Layer Mechanics Audit", "Agent 2 Invalidation/Layer Audit", "Agent 2 Exit / Take-Profit Audit", "Milestone C verdict: low accumulation sample", "Strategy Intelligence Summary", "Research only; no live config changed; no order authority changed"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}
