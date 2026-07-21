package backtest

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
	"testing"
)

func TestRunWalkForwardAnalysisChronologyAndDeterminism(t *testing.T) {
	cfg := config.Config{}
	cs := calibrationCandles(300)
	btc := map[string][]market.Candle{"1d": cs, "4h": cs, "1w": cs}
	a, e := RunWalkForwardAnalysis(cfg, btc, 3, .7, 7)
	if e != nil {
		t.Fatal(e)
	}
	b, e := RunWalkForwardAnalysis(cfg, btc, 3, .7, 7)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Splits) != len(b.Splits) {
		t.Fatal("nondeterministic")
	}
	for _, s := range a.Splits {
		if s.EvalStart-s.TrainEnd < 7 || s.TrainEnd > s.EvalStart {
			t.Fatalf("leakage %+v", s)
		}
		if s.Eval.Samples == 0 {
			t.Fatalf("empty eval %+v", s)
		}
	}
}

func TestEvaluateWalkForwardVerdictUsesEvaluationSamplesOnly(t *testing.T) {
	report := WalkForwardAnalysisReport{Enabled: true, Splits: []WalkForwardAnalysisSplit{
		{Train: CoreSignalStats{Samples: 1000, Permissions: map[agent1.Permission]int{agent1.Allowed: 1000}}, Eval: CoreSignalStats{Samples: 20, Permissions: map[agent1.Permission]int{agent1.Allowed: 2}}},
		{Train: CoreSignalStats{Samples: 1000, Permissions: map[agent1.Permission]int{agent1.Allowed: 1000}}, Eval: CoreSignalStats{Samples: 30, Permissions: map[agent1.Permission]int{agent1.Allowed: 3}}},
	}}
	got := EvaluateWalkForwardVerdict(report, 2, 50)
	if got.Status != "RESEARCH_REVIEW_REQUIRED" || got.EvaluationSamples != 50 || got.AllowedRate != 0.1 {
		t.Fatalf("verdict must use eval rows only: %+v", got)
	}
	low := EvaluateWalkForwardVerdict(report, 3, 100)
	if low.Status != "INSUFFICIENT_DATA" {
		t.Fatalf("low samples must not approve sizing: %+v", low)
	}
}
