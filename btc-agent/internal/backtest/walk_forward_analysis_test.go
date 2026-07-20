package backtest

import (
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
