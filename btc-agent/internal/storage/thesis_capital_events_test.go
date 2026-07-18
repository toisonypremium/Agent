package storage

import (
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
)

func seedThesisReservedBuy(t *testing.T, db *DB, thesisID, clientID string, reserved float64) {
	t.Helper()
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: thesisID, Symbol: "ETHUSDT", MaxExposureUSDT: 100, RemainingDCAUSDT: 100, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	d := liveguard.ManagedDesiredOrder{ThesisID: thesisID, Symbol: "ETHUSDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: reserved / 100, Notional: reserved, Source: "HERMES_OPERATOR"}
	if err := db.ReserveManagedLiveOrderWithThesis(clientID, d, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestApplyThesisBuyFillDeltaIsAtomicAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fill.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "thesis-f", "client-f", 40)
	applied, err := db.ApplyThesisBuyFillDelta("fill-1", "client-f", 15)
	if err != nil || !applied {
		t.Fatalf("first apply=%v err=%v", applied, err)
	}
	applied, err = db.ApplyThesisBuyFillDelta("fill-1", "client-f", 15)
	if err != nil || applied {
		t.Fatalf("replay apply=%v err=%v", applied, err)
	}
	applied, err = db.ApplyThesisBuyFillDelta("fill-2", "client-f", 10)
	if err != nil || !applied {
		t.Fatalf("second apply=%v err=%v", applied, err)
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-f")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReservedUSDT != 15 || got.FilledUSDT != 25 || got.RemainingDCAUSDT != 60 {
		t.Fatalf("bad ledger: %+v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applied, err = db.ApplyThesisBuyFillDelta("fill-1", "client-f", 15)
	if err != nil || applied {
		t.Fatalf("restart replay apply=%v err=%v", applied, err)
	}
}

func TestApplyThesisBuyFillDeltaFailuresDoNotMutate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "fill-fail.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-f", "client-f", 10)
	if applied, err := db.ApplyThesisBuyFillDelta("too-large", "client-f", 11); err == nil || applied {
		t.Fatal("expected over-reserved rejection")
	}
	if applied, err := db.ApplyThesisBuyFillDelta("missing", "missing", 1); err == nil || applied {
		t.Fatal("expected missing order rejection")
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-f")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReservedUSDT != 10 || got.FilledUSDT != 0 {
		t.Fatalf("failed event mutated ledger: %+v", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM thesis_capital_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed events journaled=%d", count)
	}
}

func TestApplyThesisBuyFillDeltaRejectsSellAndLegacyOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "fill-side.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveManagedLiveOrder("legacy", "", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, .1, 10, "PLANNED", live.OrderStatus{}); err != nil {
		t.Fatal(err)
	}
	if applied, err := db.ApplyThesisBuyFillDelta("legacy-fill", "legacy", 1); err == nil || applied {
		t.Fatal("expected legacy thesis rejection")
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-s", Symbol: "ETHUSDT", MaxExposureUSDT: 10, ReservedUSDT: 10, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO live_orders(client_order_id,symbol,side,status,thesis_id) VALUES('sell','ETHUSDT','SELL','PLANNED','thesis-s')`); err != nil {
		t.Fatal(err)
	}
	if applied, err := db.ApplyThesisBuyFillDelta("sell-fill", "sell", 1); err == nil || applied {
		t.Fatal("expected SELL rejection")
	}
}

func TestApplyThesisBuyFillDeltaRejectsEventKeyCollision(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "collision.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-c", "client-c", 20)
	if applied, err := db.ApplyThesisBuyFillDelta("same-key", "client-c", 5); err != nil || !applied {
		t.Fatalf("first apply=%v err=%v", applied, err)
	}
	if applied, err := db.ApplyThesisBuyFillDelta("same-key", "client-c", 6); err == nil || applied {
		t.Fatal("expected event-key payload collision")
	}
	got, err := db.ThesisCapitalLedgerByID("thesis-c")
	if err != nil || got.ReservedUSDT != 15 || got.FilledUSDT != 5 {
		t.Fatalf("collision mutated ledger: %+v err=%v", got, err)
	}
}
