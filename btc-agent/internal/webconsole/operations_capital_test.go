package webconsole

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/storage"
)

func TestRuntimeHealthUsesTypedSnapshotAndFailsClosedWhenMissing(t *testing.T) {
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	db, err := storage.Open(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRuntimeHealthSource(NewRuntimeHealthFile(filepath.Join(t.TempDir(), "missing.json")))
	missing, err := svc.RuntimeHealth()
	if err != nil {
		t.Fatal(err)
	}
	if missing.Freshness.State != "unavailable" || missing.Scheduler.State != "unavailable" {
		t.Fatalf("missing=%+v", missing)
	}

	path := filepath.Join(t.TempDir(), "health.json")
	snapshot := RuntimeHealthSnapshot{ObservedAt: now.Add(-2 * time.Minute), SchedulerCount: 1, HeartbeatState: "healthy", HeartbeatAgeSeconds: 2, LeaseInstanceID: "immutable-shadow-01", LeaseFresh: true, DatabaseState: "ok", ObserverState: "pass"}
	body, _ := json.Marshal(snapshot)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	svc.SetRuntimeHealthSource(NewRuntimeHealthFile(path))
	fresh, err := svc.RuntimeHealth()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Freshness.State != "fresh" || fresh.Scheduler.Count != 1 || fresh.Lease.InstanceID != "immutable-shadow-01" {
		t.Fatalf("fresh=%+v", fresh)
	}
}

func TestCapitalReadModelUsesThesisIDAndNeverInfersFromSymbol(t *testing.T) {
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	db, err := storage.Open(filepath.Join(t.TempDir(), "capital.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveThesisCapitalLedger(storage.ThesisCapitalLedger{ThesisID: "thesis-btc-001", Symbol: "BTC-USDT", MaxExposureUSDT: 1000, ReservedUSDT: 100, FilledUSDT: 200, RemainingDCAUSDT: 700, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	overview, err := svc.CapitalOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Currency != "USDT" || overview.ReservedUSDT != 100 || overview.FilledUSDT != 200 || overview.AvailableUSDT != 700 {
		t.Fatalf("overview=%+v", overview)
	}
	page, err := svc.ThesisCapital(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ThesisID != "thesis-btc-001" || page.Items[0].Symbol != "BTC-USDT" || page.Items[0].RemainingUSDT != 700 {
		t.Fatalf("page=%+v", page)
	}
}

func TestRuntimeHealthMarksStaleSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	db, err := storage.Open(filepath.Join(t.TempDir(), "stale.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(t.TempDir(), "health.json")
	body, _ := json.Marshal(RuntimeHealthSnapshot{ObservedAt: now.Add(-6 * time.Minute), SchedulerCount: 1, HeartbeatState: "healthy", ObserverState: "pass"})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRuntimeHealthSource(NewRuntimeHealthFile(path))
	out, err := svc.RuntimeHealth()
	if err != nil || out.Freshness.State != "stale" || out.Scheduler.State != "unavailable" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestCapitalReadModelSurfacesProjectionDriftWithoutRepair(t *testing.T) {
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	db, err := storage.Open(filepath.Join(t.TempDir(), "drift.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveThesisCapitalLedger(storage.ThesisCapitalLedger{ThesisID: "thesis-drift", Symbol: "ETH-USDT", MaxExposureUSDT: 100, ReservedUSDT: 10, FilledUSDT: 0, RemainingDCAUSDT: 90, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.CapitalOverview()
	if err != nil || out.ProjectionState != "drifted" || len(out.Issues) == 0 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-drift")
	if err != nil || ledger.ReservedUSDT != 10 {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
}

func TestRuntimeHealthArtifactIsFixedAllowlistRegularFile(t *testing.T) {
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	if _, err := NewRuntimeHealthArtifact(dir).LoadRuntimeHealth(); err == nil { t.Fatal("missing fixed artifact accepted") }
	body, _ := json.Marshal(RuntimeHealthSnapshot{ObservedAt: now, SchedulerCount: 1, HeartbeatState: "healthy", ObserverState: "pass"})
	if err := os.WriteFile(filepath.Join(dir, runtimeHealthArtifactName), body, 0600); err != nil { t.Fatal(err) }
	if _, err := NewRuntimeHealthArtifact(dir).LoadRuntimeHealth(); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(dir, runtimeHealthArtifactName)); err != nil { t.Fatal(err) }
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, runtimeHealthArtifactName)); err != nil { t.Fatal(err) }
	if _, err := NewRuntimeHealthArtifact(dir).LoadRuntimeHealth(); err == nil { t.Fatal("symlink artifact accepted") }
}
