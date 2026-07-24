package dca

import (
	"btc-agent/internal/okxassets"
	"btc-agent/internal/storage"
	"path/filepath"
	"testing"
	"time"
)

type fakeArtifactSource struct {
	a   okxassets.Artifact
	err error
}

func (f fakeArtifactSource) Load(time.Time) (okxassets.Artifact, error) { return f.a, f.err }
func TestCoordinatorOnlyAllocatesVerifiedStableArtifact(t *testing.T) {
	db, e := storage.Open(filepath.Join(t.TempDir(), "d.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	a := okxassets.Artifact{SchemaVersion: 1, Source: okxassets.SourceOKXSpotReadOnly, ObservedAt: at.Format(time.RFC3339), State: okxassets.StateVerified, Assets: []okxassets.Asset{{Currency: "USDT", Available: "100", Frozen: "0", Total: "100", ThesisLink: okxassets.ThesisNotApplicable}}}
	c := AllocationCoordinator{DB: db, Source: fakeArtifactSource{a: a}, Now: func() time.Time { return at }}
	out, e := c.ObserveAndMaybeAllocate()
	if e != nil || !out.Observed || out.Proposal != nil {
		t.Fatalf("out=%+v e=%v", out, e)
	}
	a.ObservedAt = at.Add(15 * time.Minute).Format(time.RFC3339)
	c.Source = fakeArtifactSource{a: a}
	c.Now = func() time.Time { return at.Add(15 * time.Minute) }
	out, e = c.ObserveAndMaybeAllocate()
	if e != nil || out.EpochID == 0 || !out.Applied {
		t.Fatalf("out=%+v e=%v", out, e)
	}
}
func TestCoordinatorRejectsUnavailableWithoutObservation(t *testing.T) {
	db, e := storage.Open(filepath.Join(t.TempDir(), "d.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	out, e := (AllocationCoordinator{DB: db, Source: fakeArtifactSource{err: assertErr{}}, Now: time.Now}).ObserveAndMaybeAllocate()
	if e != nil || out.Reason != "artifact_unavailable" {
		t.Fatalf("out=%+v e=%v", out, e)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "no artifact" }
