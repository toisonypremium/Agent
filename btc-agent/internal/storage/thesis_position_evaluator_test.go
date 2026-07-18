package storage

import (
	"math"
	"path/filepath"
	"testing"
)

func TestEvaluateThesisPositionLifecycleProposesReviewOnly(t *testing.T) {
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionOpen, InvalidationPrice: 90, PositionQuantity: 1, AvgEntryPrice: 100}
	got, err := EvaluateThesisPositionLifecycle(v, 89)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProposedState != ThesisPositionInvalidatedReview || !got.BlocksDCA || got.SellAuthority || !got.Changed {
		t.Fatalf("evaluation=%+v", got)
	}
}
func TestEvaluateThesisPositionLifecycleReadOnlyAboveInvalidation(t *testing.T) {
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionAccumulating, InvalidationPrice: 90, PositionQuantity: 1, AvgEntryPrice: 100}
	got, err := EvaluateThesisPositionLifecycle(v, 100)
	if err != nil || got.ProposedState != v.State || got.BlocksDCA || got.Changed || got.SellAuthority {
		t.Fatalf("evaluation=%+v err=%v", got, err)
	}
}
func TestEvaluateThesisPositionLifecycleTerminalStates(t *testing.T) {
	for _, state := range []ThesisPositionState{ThesisPositionClosed, ThesisPositionInvalidatedReview, ThesisPositionManualReview} {
		got, err := EvaluateThesisPositionLifecycle(ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: state}, 1)
		if err != nil || got.ProposedState != state || !got.BlocksDCA || got.SellAuthority {
			t.Fatalf("state=%s eval=%+v err=%v", state, got, err)
		}
	}
}
func TestEvaluateThesisPositionLifecycleRejectsInvalidPrice(t *testing.T) {
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionOpen}
	for _, price := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := EvaluateThesisPositionLifecycle(v, price); err == nil {
			t.Fatalf("price %v accepted", price)
		}
	}
}
func TestEvaluateThesisPositionLifecycleDBReadOnly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "eval.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionOpen, InvalidationPrice: 90, PositionQuantity: 1, AvgEntryPrice: 100}
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	got, err := db.EvaluateThesisPositionLifecycleByID("t", 80)
	if err != nil || got.ProposedState != ThesisPositionInvalidatedReview {
		t.Fatalf("eval=%+v err=%v", got, err)
	}
	stored, err := db.ThesisPositionLifecycleByID("t")
	if err != nil || stored.State != ThesisPositionOpen {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
}
