package storage

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/config"
)

func TestCapitalAuthoritySnapshotSeparatesCapacityAndPermission(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "capital.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.UpdateEquityRiskState(1000, time.Now()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.Portfolio.ReserveCashRatio = .1
	cfg.HermesOperator.MaxPortfolioExposureUSDT = 700
	got, err := db.BuildCapitalAuthoritySnapshot(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.ReserveRequiredUSDT != 100 || got.Permission != "BLOCKED" || got.ConditionalCapacityUSDT <= 0 {
		t.Fatalf("snapshot=%+v", got)
	}
}

func TestCapitalAuthoritySnapshotFailsClosedWithoutEquity(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "capital-missing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.BuildCapitalAuthoritySnapshot(config.Config{}, time.Now())
	if err == nil || got.Permission != "UNKNOWN" || len(got.Blockers) == 0 {
		t.Fatalf("snapshot=%+v err=%v", got, err)
	}
}
