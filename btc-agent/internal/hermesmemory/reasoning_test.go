package hermesmemory

import (
	"testing"
	"time"
)

func TestReasoningMarksContradictionAsUnknown(t *testing.T) {
	e := BuildEpisode(Situation{Authority: "BLOCKED"}, "WAIT", []string{"gate blocked"}, nil, nil)
	r := BuildReasoning(e, Context{CalibratedConfidence: .4, Contradictions: []string{"similar regime differed"}}, []string{"wait"})
	if len(r.Unknowns) == 0 || r.Authority != "deterministic_engine_only" {
		t.Fatalf("bad reasoning %+v", r)
	}
}
func TestPredictionScoreClampsAndScores(t *testing.T) {
	p := ScorePrediction(Prediction{Symbol: "ethusdt", Confidence: 2, ExpectedReturn: .02}, -.03, time.Now())
	if p.Symbol != "ETHUSDT" || p.Confidence != 1 || p.Status != "SCORED" || p.SquaredError == nil || *p.SquaredError <= 0 {
		t.Fatalf("bad score %+v", p)
	}
}
