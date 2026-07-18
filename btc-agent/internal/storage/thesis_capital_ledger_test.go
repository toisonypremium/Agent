package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestThesisCapitalLedgerValidation(t *testing.T) {
	base := ThesisCapitalLedger{ThesisID: "thesis-1", Symbol: "ETHUSDT", MaxExposureUSDT: 100, Status: "ACCUMULATING"}
	if err := ValidateThesisCapitalLedger(base); err != nil {
		t.Fatal(err)
	}
	base.ReservedUSDT, base.FilledUSDT, base.RemainingDCAUSDT = 50, 30, 21
	if err := ValidateThesisCapitalLedger(base); err == nil {
		t.Fatal("expected budget overflow rejection")
	}
	base.ReservedUSDT, base.FilledUSDT, base.RemainingDCAUSDT = -1, 0, 0
	if err := ValidateThesisCapitalLedger(base); err == nil {
		t.Fatal("expected negative reserve rejection")
	}
}

func TestThesisCapitalLedgerRoundTripAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thesis-ledger.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ledger := ThesisCapitalLedger{ThesisID: " thesis-1 ", Symbol: "ethusdt", MaxExposureUSDT: 100, ReservedUSDT: 10, FilledUSDT: 20, RemainingDCAUSDT: 70, Status: "accumulating"}
	if err := db.SaveThesisCapitalLedger(ledger); err != nil {
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
	got, err := db.ThesisCapitalLedgerByID("thesis-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "ETHUSDT" || got.Status != "ACCUMULATING" || got.ReservedUSDT != 10 || got.FilledUSDT != 20 || got.RemainingDCAUSDT != 70 {
		t.Fatalf("bad ledger: %+v", got)
	}
}

func TestLegacyDatabaseGetsAdditiveThesisSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE live_orders(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, type TEXT, price REAL, quantity REAL, notional REAL, status TEXT, submitted_at INTEGER, updated_at INTEGER, payload_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO live_orders(client_order_id,symbol,status) VALUES('legacy-1','ETHUSDT','FILLED')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM thesis_capital_ledgers`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	var thesisID sql.NullString
	if err := db.QueryRow(`SELECT thesis_id FROM live_orders WHERE client_order_id='legacy-1'`).Scan(&thesisID); err != nil {
		t.Fatal(err)
	}
	if thesisID.Valid {
		t.Fatalf("legacy order received inferred thesis id %q", thesisID.String)
	}
}
