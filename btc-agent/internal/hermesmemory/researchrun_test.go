package hermesmemory

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func bars() []DatasetBar {
	t := time.Unix(1000, 0)
	return []DatasetBar{{t, 10, 12, 9, 11, 100}, {t.Add(time.Hour), 11, 13, 10, 12, 120}}
}
func TestDatasetHashStable(t *testing.T) {
	a, e := BuildDatasetRecord("btcusdt", "1H", "canonical", bars(), 1)
	b, e2 := BuildDatasetRecord("BTCUSDT", "1h", "canonical", bars(), 1)
	if e != nil || e2 != nil || a.ContentHash != b.ContentHash || a.DatasetID != b.DatasetID {
		t.Fatalf("unstable hash %v %v", e, e2)
	}
}
func TestDatasetRejectsDirtyOHLC(t *testing.T) {
	b := bars()
	b[0].High = 8
	if _, e := BuildDatasetRecord("BTC", "1h", "x", b, 1); e == nil {
		t.Fatal("dirty OHLC accepted")
	}
}
func TestDatasetRejectsDuplicateTime(t *testing.T) {
	b := bars()
	b[1].Timestamp = b[0].Timestamp
	if e := ValidateDatasetBars(b, time.Time{}); e == nil {
		t.Fatal("duplicate timestamp accepted")
	}
}
func TestDatasetRejectsLookahead(t *testing.T) {
	b := bars()
	if e := ValidateDatasetBars(b, b[0].Timestamp); e == nil {
		t.Fatal("lookahead accepted")
	}
}
func TestResearchRunRejectsAuthority(t *testing.T) {
	r := NormalizeResearchRun(ResearchRun{HypothesisID: "h", DatasetID: "d", DatasetHash: "x", CodeHash: "c", ConfigHash: "q", EngineVersion: "v", Authority: "execution"})
	if r.Authority == "research_only" {
		t.Fatal("authority silently changed")
	}
}

func TestEnsureBaselineResearchPlanSingleConnectionDoesNotDeadlock(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "hermes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for i, symbol := range []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"} {
		datasetBars := bars()
		for j := range datasetBars {
			datasetBars[j].Open += float64(i)
			datasetBars[j].High += float64(i)
			datasetBars[j].Low += float64(i)
			datasetBars[j].Close += float64(i)
		}
		dataset, buildErr := BuildDatasetRecord(symbol, "1h", "canonical_sqlite_candles_closed", datasetBars, 1)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		if saveErr := SaveDataset(db, dataset); saveErr != nil {
			t.Fatal(saveErr)
		}
	}

	done := make(chan error, 1)
	go func() { done <- EnsureBaselineResearchPlan(db, "episode-single-connection") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureBaselineResearchPlan deadlocked with one SQLite connection")
	}

	var runs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hermes_research_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 3 {
		t.Fatalf("runs=%d want=3", runs)
	}
}
