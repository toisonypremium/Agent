package microstructure

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCalibrationLearnsBoundedAndPersists(t *testing.T) {
	s := NewCalibrationState()
	now := time.Unix(100000, 0).UTC()
	for i := 0; i < 8; i++ {
		s.ObserveAndLearn(now, map[string]MMFootprintSignal{"BTCUSDT": {Verdict: "POSSIBLE_ACCUMULATION", PriceLatest: 100}})
		now = now.Add(25 * time.Hour)
		s.ObserveAndLearn(now, map[string]MMFootprintSignal{"BTCUSDT": {Verdict: "NO_SIGNAL", PriceLatest: 99}})
		now = now.Add(time.Hour)
	}
	c := s.Symbols["BTCUSDT"]
	if c.TakerAnomalyZ != 1.55 {
		t.Fatalf("threshold=%v state=%+v", c.TakerAnomalyZ, c)
	}
	b, _ := json.Marshal(s)
	var restored CalibrationState
	if json.Unmarshal(b, &restored) != nil || restored.Symbols["BTCUSDT"].TakerAnomalyZ != 1.55 {
		t.Fatalf("not persisted: %s", b)
	}
	for i := 0; i < 400; i++ {
		c.BatchResolved = 8
		c.BatchSuccesses = 0
		s.ObserveAndLearn(now, map[string]MMFootprintSignal{"BTCUSDT": {PriceLatest: 99}})
	}
	if c.TakerAnomalyZ > MaxTakerAnomalyZ {
		t.Fatal("threshold exceeded bound")
	}
}
func TestCalibrationDoesNotOverlapPendingObservation(t *testing.T) {
	s := NewCalibrationState()
	now := time.Now().UTC()
	sig := map[string]MMFootprintSignal{"ETHUSDT": {Verdict: "POSSIBLE_ACCUMULATION", PriceLatest: 100}}
	s.ObserveAndLearn(now, sig)
	first := s.Symbols["ETHUSDT"].Pending.ObservedAt
	s.ObserveAndLearn(now.Add(time.Hour), sig)
	if !s.Symbols["ETHUSDT"].Pending.ObservedAt.Equal(first) {
		t.Fatal("pending observation overlapped")
	}
}
