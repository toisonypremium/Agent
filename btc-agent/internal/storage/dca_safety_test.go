package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDCASafetyAutoHaltsAfterThreeErrorsOrTwoStaleEpochs(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "d.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	now := time.Now()
	for i := 0; i < 3; i++ {
		s, h, e := d.RecordDCASafetyCycle(true, false, "three_consecutive_errors", now)
		if e != nil || h != (i == 2) || s.ConsecutiveErrors != i+1 {
			t.Fatalf("i=%d s=%+v h=%v e=%v", i, s, h, e)
		}
	}
	halted, e := d.IsHalted()
	if e != nil || !halted {
		t.Fatal("halt missing")
	}
	d2, e := Open(filepath.Join(t.TempDir(), "s.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer d2.Close()
	for i := 0; i < 2; i++ {
		_, h, e := d2.RecordDCASafetyCycle(false, true, "observer_stale", now)
		if e != nil || h != (i == 1) {
			t.Fatalf("stale %d %v %v", i, h, e)
		}
	}
}
