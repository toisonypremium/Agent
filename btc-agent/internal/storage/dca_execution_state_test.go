package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDCAExecutionGlobalCapOnlyRampsAfterUniqueReconciledFill(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "d.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	state, e := d.DCAExecutionState()
	if e != nil || state.GlobalCapPercent != 20 {
		t.Fatalf("%+v %v", state, e)
	}
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	for i, want := range []float64{40, 60, 80, 100, 100} {
		state, applied, e := d.AdvanceDCAExposureCapAfterReconciledFill("fill-"+string(rune('a'+i)), at)
		if e != nil || !applied || state.GlobalCapPercent != want {
			t.Fatalf("i=%d state=%+v applied=%v err=%v", i, state, applied, e)
		}
	}
	state, applied, e := d.AdvanceDCAExposureCapAfterReconciledFill("fill-a", at)
	if e != nil || applied || state.GlobalCapPercent != 100 {
		t.Fatalf("replay=%+v %v %v", state, applied, e)
	}
}
