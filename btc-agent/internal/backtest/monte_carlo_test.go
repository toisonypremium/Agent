package backtest

import "testing"

func mcWalk(pnls ...float64) WalkForwardReport {
	a := map[string]AssetSimStats{}
	for i, p := range pnls {
		a[string(rune('A'+i))] = AssetSimStats{OrdersFilled: 1, FinalPnL: p}
	}
	return WalkForwardReport{Enabled: true, Splits: []WalkForwardSplit{{Eval: Agent2Simulation{Enabled: true, Assets: a}}}}
}
func TestMonteCarloDeterministic(t *testing.T) {
	w := mcWalk(10, 8, 6, -2, -1)
	a, e := RunMonteCarloRobustness(w, 1000, 42)
	if e != nil {
		t.Fatal(e)
	}
	b, e := RunMonteCarloRobustness(w, 1000, 42)
	if e != nil {
		t.Fatal(e)
	}
	if a != b {
		t.Fatalf("not deterministic: %+v %+v", a, b)
	}
	if !a.Enabled || a.Samples != 5 || a.MedianFinalPnL <= 0 {
		t.Fatalf("bad robust report: %+v", a)
	}
}
func TestMonteCarloFragile(t *testing.T) {
	r, e := RunMonteCarloRobustness(mcWalk(-10, -8, -6, 1, 2), 1000, 7)
	if e != nil {
		t.Fatal(e)
	}
	if r.Verdict != "FRAGILE" || r.ProbabilityOfLoss < .5 {
		t.Fatalf("bad fragile verdict: %+v", r)
	}
}
func TestMonteCarloNeedsSamples(t *testing.T) {
	if _, e := RunMonteCarloRobustness(mcWalk(1, 2), 1000, 1); e == nil {
		t.Fatal("expected sample error")
	}
}
