package storage

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
)

func TestUpdatePaperOrderStatusAndCounts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order := paperStorageOrder("paper-1", "ETHUSDT", 1, 100)
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdatePaperOrderStatus(order.ID, "FILLED", "filled in test"); err != nil {
		t.Fatal(err)
	}
	counts, err := db.PaperOrderStatusCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["FILLED"] != 1 || counts["OPEN"] != 0 {
		t.Fatalf("bad counts: %+v", counts)
	}
	open, err := db.OpenPaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("filled order should not be open: %+v", open)
	}
}

func TestSaveOrdersSkipsEquivalentOpenPaperOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := paperStorageOrder("paper-1", "ETHUSDT", 1, 100)
	second := paperStorageOrder("paper-2", "ethusdt", 1, 100)
	if err := db.SaveOrders([]agent2.PaperOrder{first}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOrders([]agent2.PaperOrder{second}); err != nil {
		t.Fatal(err)
	}
	open, err := db.OpenPaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != first.ID {
		t.Fatalf("expected duplicate skipped, got %+v", open)
	}
}

func paperStorageOrder(id, symbol string, layer int, price float64) agent2.PaperOrder {
	now := time.Unix(1700000000, 0)
	return agent2.PaperOrder{ID: id, Timestamp: now, Symbol: symbol, Side: "BUY", Layer: layer, Price: price, Quantity: 1, Notional: price, Status: "OPEN", ExpiresAt: now.Add(time.Hour), InvalidationPrice: price * 0.9, Reason: "test"}
}
