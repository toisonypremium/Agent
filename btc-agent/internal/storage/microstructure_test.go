package storage

import (
	"testing"
	"time"

	"btc-agent/internal/microstructure"
)

func TestMicrostructureSnapshotsSaveLoadAndPrune(t *testing.T) {
	db, err := Open(t.TempDir() + "/micro.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	old := microstructure.Snapshot{Symbol: "BTCUSDT", Timestamp: time.Unix(100, 0), Source: "test", Health: microstructure.Health{Fresh: true}, Signals: microstructure.Signals{BuyPressure: "BUY_DOMINANT"}}
	now := old.Timestamp.AddDate(0, 0, 2)
	latest := microstructure.Snapshot{Symbol: "ETHUSDT", Timestamp: now, Source: "test", Health: microstructure.Health{Fresh: false, Blockers: []string{"stale"}}, Signals: microstructure.Signals{BuyPressure: "SELL_DOMINANT"}}
	if err := db.SaveMicrostructureSnapshots([]microstructure.Snapshot{old, latest}); err != nil {
		t.Fatalf("save snapshots: %v", err)
	}
	loaded, err := db.LoadMicrostructureSnapshots("BTCUSDT", 10)
	if err != nil {
		t.Fatalf("load snapshots: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Symbol != "BTCUSDT" || !loaded[0].Health.Fresh {
		t.Fatalf("unexpected loaded snapshots: %+v", loaded)
	}
	latestRows, err := db.LatestMicrostructureSnapshots([]string{"BTCUSDT", "ETHUSDT"})
	if err != nil {
		t.Fatalf("latest snapshots: %v", err)
	}
	if len(latestRows) != 2 {
		t.Fatalf("expected 2 latest snapshots, got %d", len(latestRows))
	}
	result, err := db.PruneMaintenance(MaintenanceConfig{EventRetentionDays: 1}, now)
	if err != nil {
		t.Fatalf("maintenance: %v", err)
	}
	if result.MicrostructureDeleted != 1 {
		t.Fatalf("expected 1 microstructure snapshot pruned, got %d", result.MicrostructureDeleted)
	}
}
