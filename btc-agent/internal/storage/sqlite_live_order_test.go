package storage

import (
	"path/filepath"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestLiveOrderStorageRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SaveLiveOrderFromParams("client-1", "order-1", "ETH-USDT", "ETHUSDT", "BUY", "limit", 2000, 0.01, 20, live.StatusLiveOpen); err != nil {
		t.Fatalf("save live order: %v", err)
	}

	open, err := db.OpenLiveOrders()
	if err != nil {
		t.Fatalf("open live orders: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("open len=%d want 1", len(open))
	}
	if open[0].ClientOrderID != "client-1" || open[0].OrderID != "order-1" || open[0].Status != live.StatusLiveOpen {
		t.Fatalf("bad open order: %+v", open[0])
	}

	status := live.OrderStatus{
		InstID:            "ETH-USDT",
		OrderID:           "order-1",
		ClientOrderID:     "client-1",
		State:             "filled",
		Status:            live.StatusFilled,
		Side:              "buy",
		OrderType:         "limit",
		Price:             2000,
		Quantity:          0.01,
		FilledQuantity:    0.01,
		AvgPrice:          1999,
		AccumulatedFillSz: 0.01,
		Fee:               -0.001,
		UpdatedAt:         1700000000000,
	}
	if err := db.SaveLiveOrderStatus(status); err != nil {
		t.Fatalf("save live order status: %v", err)
	}
	if err := db.SaveLiveOrderEvent(status); err != nil {
		t.Fatalf("save live order event: %v", err)
	}

	open, err = db.OpenLiveOrders()
	if err != nil {
		t.Fatalf("open after filled: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("filled order should no longer be open: %+v", open)
	}

	var savedStatus string
	var payload string
	if err := db.QueryRow(`SELECT status, payload_json FROM live_orders WHERE client_order_id=?`, "client-1").Scan(&savedStatus, &payload); err != nil {
		t.Fatalf("query saved order: %v", err)
	}
	if savedStatus != live.StatusFilled || payload == "" {
		t.Fatalf("bad saved status=%q payload=%q", savedStatus, payload)
	}

	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM live_order_events WHERE client_order_id=? AND status=?`, "client-1", live.StatusFilled).Scan(&eventCount); err != nil {
		t.Fatalf("query event count: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("event count=%d want 1", eventCount)
	}
}

func TestSaveLiveOrderStatusCanMatchByOrderID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SaveLiveOrderFromParams("client-2", "order-2", "ETH-USDT", "ETHUSDT", "BUY", "limit", 2000, 0.01, 20, live.StatusLiveOpen); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLiveOrderStatus(live.OrderStatus{OrderID: "order-2", Status: live.StatusCanceled}); err != nil {
		t.Fatalf("save by order id: %v", err)
	}

	var savedStatus string
	if err := db.QueryRow(`SELECT status FROM live_orders WHERE client_order_id=?`, "client-2").Scan(&savedStatus); err != nil {
		t.Fatal(err)
	}
	if savedStatus != live.StatusCanceled {
		t.Fatalf("status=%s want %s", savedStatus, live.StatusCanceled)
	}
}

func TestHaltStatusStorage(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 1. Default should fail closed to halted=true
	halted, err := db.IsHalted()
	if err != nil {
		t.Fatal(err)
	}
	if !halted {
		t.Fatal("expected default halt status to be true")
	}

	// 2. Set to true
	if err := db.SetHaltStatus(true); err != nil {
		t.Fatal(err)
	}
	halted, err = db.IsHalted()
	if err != nil {
		t.Fatal(err)
	}
	if !halted {
		t.Fatal("expected halt status to be true after setting to true")
	}

	// 3. Set back to false
	if err := db.SetHaltStatus(false); err != nil {
		t.Fatal(err)
	}
	halted, err = db.IsHalted()
	if err != nil {
		t.Fatal(err)
	}
	if halted {
		t.Fatal("expected halt status to be false after resetting to false")
	}
}
