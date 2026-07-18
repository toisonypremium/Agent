package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestThesisPositionLifecycleRoundTripRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lifecycle.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v := ThesisPositionLifecycle{ThesisID: " thesis-l ", Symbol: "ethusdt", State: ThesisPositionPlanned, InvalidationPrice: 80, PrimaryTargetPrice: 140, Version: 1}
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.ThesisPositionLifecycleByID("thesis-l")
	if err != nil || got.Symbol != "ETHUSDT" || got.State != ThesisPositionPlanned {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
func TestThesisPositionLifecycleTransitionsAndDCA(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "states.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionPlanned}
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	v.State = ThesisPositionAccumulating
	v.PositionQuantity = 1
	v.AvgEntryPrice = 100
	v.OpenedAt = time.Now()
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	if !ThesisPositionStateAllowsDCA(v.State) {
		t.Fatal("accumulating should permit DCA contract")
	}
	v.State = ThesisPositionInvalidatedReview
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	if ThesisPositionStateAllowsDCA(v.State) {
		t.Fatal("invalidated review must block DCA")
	}
	v.State = ThesisPositionAccumulating
	if err := db.SaveThesisPositionLifecycle(v); err == nil {
		t.Fatal("invalidated lifecycle reopened automatically")
	}
}
func TestThesisPositionLifecycleClosedAndIdentityImmutable(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "closed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionPlanned}
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	v.Symbol = "BTCUSDT"
	if err := db.SaveThesisPositionLifecycle(v); err == nil {
		t.Fatal("symbol changed")
	}
	v.Symbol = "ETHUSDT"
	v.State = ThesisPositionClosed
	if err := db.SaveThesisPositionLifecycle(v); err != nil {
		t.Fatal(err)
	}
	v.State = ThesisPositionPlanned
	if err := db.SaveThesisPositionLifecycle(v); err == nil {
		t.Fatal("closed lifecycle reopened")
	}
}
func TestValidateThesisPositionLifecycleRejectsInvalidNumbers(t *testing.T) {
	v := ThesisPositionLifecycle{ThesisID: "t", Symbol: "ETHUSDT", State: ThesisPositionOpen, PositionQuantity: 1}
	if err := ValidateThesisPositionLifecycle(v); err == nil {
		t.Fatal("quantity without basis accepted")
	}
}
