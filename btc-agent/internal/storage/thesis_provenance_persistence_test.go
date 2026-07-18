package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestAtomicFillPersistsThesisProvenanceAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provenance.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedThesisReservedBuy(t, db, "thesis-prov", "order-prov", 20)
	e := live.LivePositionEvent{Timestamp: 100, ClientOrderID: "order-prov", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20, Status: live.StatusFilled}
	snap := live.LiveFillSnapshot{ClientOrderID: "order-prov", InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200, UpdatedAt: 100}
	pos, applied, err := db.ApplyReconciledLiveFill(e, snap, "prov-fill")
	if err != nil || !applied || pos.ThesisID != "thesis-prov" {
		t.Fatalf("pos=%+v applied=%v err=%v", pos, applied, err)
	}
	fill, found, err := db.LiveFillSnapshot("order-prov", "")
	if err != nil || !found || fill.ThesisID != "thesis-prov" {
		t.Fatalf("fill=%+v found=%v err=%v", fill, found, err)
	}
	var eventThesis string
	if err := db.QueryRow(`SELECT thesis_id FROM live_position_events WHERE client_order_id='order-prov'`).Scan(&eventThesis); err != nil || eventThesis != "thesis-prov" {
		t.Fatalf("event thesis=%q err=%v", eventThesis, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	positions, err := db.LivePositions()
	if err != nil || len(positions) != 1 || positions[0].ThesisID != "thesis-prov" {
		t.Fatalf("positions=%+v err=%v", positions, err)
	}
}

func TestAtomicFillThesisMismatchRollsBack(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "prov-conflict.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedThesisReservedBuy(t, db, "thesis-good", "order-good", 20)
	e := live.LivePositionEvent{ThesisID: "thesis-bad", Timestamp: 1, ClientOrderID: "order-good", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 200, NotionalDelta: 20}
	s := live.LiveFillSnapshot{ClientOrderID: "order-good", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1, AvgPrice: 200}
	if _, applied, err := db.ApplyReconciledLiveFill(e, s, "conflict"); err == nil || applied {
		t.Fatal("expected provenance conflict")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_position_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

func TestLegacyFillAndPositionProvenanceRemainNull(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "prov-legacy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveLiveFillSnapshot(live.LiveFillSnapshot{ClientOrderID: "legacy", Symbol: "ETHUSDT", Side: "BUY", FilledQuantity: .1}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: 1, ClientOrderID: "legacy", Symbol: "ETHUSDT", Side: "BUY", DeltaQuantity: .1, FillPrice: 100, NotionalDelta: 10}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`SELECT thesis_id FROM live_fills WHERE client_order_id='legacy'`, `SELECT thesis_id FROM live_position_events WHERE client_order_id='legacy'`} {
		var v sql.NullString
		if err := db.QueryRow(q).Scan(&v); err != nil || v.Valid {
			t.Fatalf("legacy provenance=%+v err=%v", v, err)
		}
	}
}

func TestHermesHoldingPreservesAndRejectsThesisConflict(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "holding-prov.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := HermesManagedHolding{ThesisID: "thesis-h", Symbol: "ETHUSDT", InstID: "ETH-USDT", Quantity: 1, AvgEntryPrice: 100, Source: "TEST"}
	if err := db.SaveHermesManagedHolding(h); err != nil {
		t.Fatal(err)
	}
	h.ThesisID = ""
	h.Quantity = 2
	if err := db.SaveHermesManagedHolding(h); err != nil {
		t.Fatal(err)
	}
	got, err := db.HermesManagedHoldings()
	if err != nil || len(got) != 1 || got[0].ThesisID != "thesis-h" {
		t.Fatalf("holdings=%+v err=%v", got, err)
	}
	h.ThesisID = "other"
	if err := db.SaveHermesManagedHolding(h); err == nil {
		t.Fatal("expected holding thesis conflict")
	}
}
