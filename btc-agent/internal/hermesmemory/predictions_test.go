package hermesmemory

import (
	"testing"
	"time"
)

func TestNewPredictionRejectsUnknownHorizon(t *testing.T) {
	if _, _, e := NewPrediction("e", "ETHUSDT", "9d", "WATCH", 100, 0, .5, time.Now()); e == nil {
		t.Fatal("unknown horizon accepted")
	}
}
func TestCalibrationQualityThresholds(t *testing.T) {
	p, d, e := NewPrediction("e", "ethusdt", "1d", "WATCH", 100, .02, 2, time.Now())
	if e != nil || p.Confidence != 1 || !d.After(p.CreatedAt) {
		t.Fatalf("bad prediction %+v %v", p, e)
	}
}
func TestScorePredictionSquaredError(t *testing.T) {
	p := ScorePrediction(Prediction{ExpectedReturn: .1}, -.1, time.Now())
	if p.SquaredError == nil || mathAbs(*p.SquaredError-.04) > .000001 {
		t.Fatalf("bad squared error %+v", p)
	}
}
func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
