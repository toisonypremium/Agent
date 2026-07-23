package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	_ "modernc.org/sqlite"
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
	closedAt := order.Timestamp.Add(5 * time.Minute)
	if err := db.UpdatePaperOrderStatusAt(order.ID, "FILLED", "filled in test", closedAt); err != nil {
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
	if err := db.UpdatePaperOrderStatusAt(order.ID, "FILLED", "before creation", order.Timestamp.Add(-time.Second)); err == nil {
		t.Fatal("terminal time before creation must be rejected")
	}
	orders, err := db.PaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || !orders[0].ClosedAt.Equal(closedAt) || orders[0].Reason != "filled in test" {
		t.Fatalf("terminal evidence not retained: %+v", orders)
	}
	if err := db.UpdatePaperOrderStatusAt(order.ID, "CANCELLED", "replay", closedAt.Add(time.Minute)); err == nil {
		t.Fatal("terminal replay must not rewrite lifecycle evidence")
	}
}

func TestSaveOrdersRejectsNonOpenLifecycleInjection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order := paperStorageOrder("paper-terminal-injection", "ETHUSDT", 1, 100)
	order.Status = "FILLED"
	order.ClosedAt = order.Timestamp.Add(time.Minute)
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err == nil {
		t.Fatal("terminal paper order injection must be rejected")
	}
	orders, err := db.PaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 {
		t.Fatalf("rejected lifecycle injection persisted: %+v", orders)
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

func TestUpdatePaperOrderStatusAtRejectsInvalidTransition(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order := paperStorageOrder("paper-invalid", "ETHUSDT", 1, 100)
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"OPEN", "PENDING_REVIEW", ""} {
		if err := db.UpdatePaperOrderStatusAt(order.ID, status, "bad", order.Timestamp.Add(time.Minute)); err == nil {
			t.Fatalf("status %q must be rejected", status)
		}
	}
	orders, err := db.PaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].Status != "OPEN" || !orders[0].ClosedAt.IsZero() {
		t.Fatalf("invalid transition mutated order: %+v", orders)
	}
}

func TestPaperOrdersPreservesOpenZeroClosedAt(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order := paperStorageOrder("paper-open", "ETHUSDT", 1, 100)
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatal(err)
	}
	orders, err := db.PaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || !orders[0].ClosedAt.IsZero() {
		t.Fatalf("open order has unexpected terminal timestamp: %+v", orders)
	}
}

func TestOpenMigratesLegacyPaperOrdersAndKeepsTerminalTimestampZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE paper_orders(id TEXT PRIMARY KEY, timestamp INTEGER, symbol TEXT, side TEXT, layer INTEGER, price REAL, quantity REAL, notional REAL, status TEXT, expires_at INTEGER, invalidation_price REAL, reason TEXT)`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO paper_orders(id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason) VALUES('legacy-filled',100,'ETHUSDT','BUY',1,100,1,100,'FILLED',200,90,'legacy')`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	orders, err := db.PaperOrders()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].Status != "FILLED" || !orders[0].ClosedAt.IsZero() {
		t.Fatalf("legacy migration changed lifecycle evidence: %+v", orders)
	}
}
