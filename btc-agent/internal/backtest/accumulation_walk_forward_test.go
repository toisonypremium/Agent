package backtest

import "testing"

func TestRunAccumulationWalkForwardUsesEmbargoedEvaluationRanges(t *testing.T) {
	got, err := RunAccumulationWalkForward("BTCUSDT", phaseAuditCandles(360), []int{1, 3, 7, 14}, 3, 0.6, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Splits) != 3 || got.SizingExpansionAllowed {
		t.Fatalf("unexpected report: %+v", got)
	}
	for _, split := range got.Splits {
		if split.Embargo < 7 || split.EvalStart >= split.EvalEnd {
			t.Fatalf("invalid embargoed eval range: %+v", split)
		}
	}
}

func TestRunAccumulationWalkForwardLowDataDoesNotApproveSizing(t *testing.T) {
	got, err := RunAccumulationWalkForward("BTCUSDT", phaseAuditCandles(300), []int{1, 3, 7}, 3, 0.6, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "INSUFFICIENT_DATA" || got.SizingExpansionAllowed {
		t.Fatalf("low evidence must not approve sizing: %+v", got)
	}
}
