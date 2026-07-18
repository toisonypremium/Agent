package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"btc-agent/internal/liveguard"
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

func TestThesisReservationTransfersDCAToReservedAtomically(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reserve.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-r", Symbol: "ETHUSDT", MaxExposureUSDT: 100, RemainingDCAUSDT: 100, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	desired := liveguard.ManagedDesiredOrder{ThesisID: "thesis-r", Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: .2, Notional: 20, Source: "HERMES_OPERATOR"}
	if err := db.ReserveManagedLiveOrderWithThesis("client-r", desired, "test"); err != nil {
		t.Fatal(err)
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-r")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReservedUSDT != 20 || got.RemainingDCAUSDT != 80 || got.FilledUSDT != 0 {
		t.Fatalf("bad transfer: %+v", got)
	}
	var thesisID string
	if err := db.QueryRow(`SELECT thesis_id FROM live_orders WHERE client_order_id='client-r'`).Scan(&thesisID); err != nil || thesisID != "thesis-r" {
		t.Fatalf("order thesis_id=%q err=%v", thesisID, err)
	}
}

func TestThesisReservationFailuresLeaveLedgerUnchanged(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reserve-fail.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-r", Symbol: "ETHUSDT", MaxExposureUSDT: 100, RemainingDCAUSDT: 10, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	base := liveguard.ManagedDesiredOrder{ThesisID: "thesis-r", Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: .2, Notional: 20, Source: "HERMES_OPERATOR"}
	if err := db.ReserveManagedLiveOrderWithThesis("too-much", base, "test"); err == nil {
		t.Fatal("expected insufficient budget")
	}
	base.Symbol = "BTCUSDT"
	if err := db.ReserveManagedLiveOrderWithThesis("wrong-symbol", base, "test"); err == nil {
		t.Fatal("expected symbol mismatch")
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-r")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReservedUSDT != 0 || got.RemainingDCAUSDT != 10 {
		t.Fatalf("ledger mutated on failed reserve: %+v", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_orders WHERE thesis_id='thesis-r'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed reserve inserted %d orders", count)
	}
}

func TestThesisReservationDuplicateRollsBackBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duplicate.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-d", Symbol: "ETHUSDT", MaxExposureUSDT: 100, RemainingDCAUSDT: 100, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	desired := liveguard.ManagedDesiredOrder{ThesisID: "thesis-d", Symbol: "ETHUSDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: .2, Notional: 20, Source: "HERMES_OPERATOR"}
	if err := db.ReserveManagedLiveOrderWithThesis("client-a", desired, "first"); err != nil {
		t.Fatal(err)
	}
	if err := db.ReserveManagedLiveOrderWithThesis("client-b", desired, "duplicate"); err == nil {
		t.Fatal("expected duplicate logical reservation")
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-d")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReservedUSDT != 20 || got.RemainingDCAUSDT != 80 {
		t.Fatalf("duplicate changed budget: %+v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = db.ThesisCapitalLedgerByID("thesis-d")
	if err != nil || got.ReservedUSDT != 20 || got.RemainingDCAUSDT != 80 {
		t.Fatalf("reservation did not survive restart: %+v err=%v", got, err)
	}
}

func TestThesisReservationMissingOrClosedFailsWithoutOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "missing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	desired := liveguard.ManagedDesiredOrder{ThesisID: "missing", Symbol: "ETHUSDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: .1, Notional: 10, Source: "HERMES_OPERATOR"}
	if err := db.ReserveManagedLiveOrderWithThesis("missing", desired, "test"); err == nil {
		t.Fatal("expected missing thesis failure")
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "closed", Symbol: "ETHUSDT", MaxExposureUSDT: 10, RemainingDCAUSDT: 10, Status: "CLOSED"}); err != nil {
		t.Fatal(err)
	}
	desired.ThesisID = "closed"
	if err := db.ReserveManagedLiveOrderWithThesis("closed", desired, "test"); err == nil {
		t.Fatal("expected closed thesis failure")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_orders`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed reservations inserted %d orders", count)
	}
}

func TestLegacyReservationDoesNotRequireOrMutateThesisLedger(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy-reserve.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	desired := liveguard.ManagedDesiredOrder{Symbol: "ETHUSDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: .1, Notional: 10, Source: "legacy"}
	if err := db.ReserveManagedLiveOrder("legacy-client", desired, "legacy"); err != nil {
		t.Fatal(err)
	}
	var ledgers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM thesis_capital_ledgers`).Scan(&ledgers); err != nil {
		t.Fatal(err)
	}
	if ledgers != 0 {
		t.Fatalf("legacy reserve created %d thesis ledgers", ledgers)
	}
	var thesisID sql.NullString
	if err := db.QueryRow(`SELECT thesis_id FROM live_orders WHERE client_order_id='legacy-client'`).Scan(&thesisID); err != nil {
		t.Fatal(err)
	}
	if thesisID.Valid {
		t.Fatalf("legacy reserve inferred thesis %q", thesisID.String)
	}
}

func TestThesisCapitalLedgerRejectsSymbolChange(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "immutable-symbol.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := ThesisCapitalLedger{ThesisID: "thesis-i", Symbol: "ETHUSDT", MaxExposureUSDT: 10, RemainingDCAUSDT: 10, Status: "ACCUMULATING"}
	if err := db.SaveThesisCapitalLedger(base); err != nil {
		t.Fatal(err)
	}
	base.Symbol = "BTCUSDT"
	if err := db.SaveThesisCapitalLedger(base); err == nil {
		t.Fatal("expected immutable symbol rejection")
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-i")
	if err != nil || got.Symbol != "ETHUSDT" {
		t.Fatalf("symbol changed: %+v err=%v", got, err)
	}
}
