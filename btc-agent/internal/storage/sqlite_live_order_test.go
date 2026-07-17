package storage

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
)

func TestSaveOrdersPreservesClosedPaperOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Unix(1700000000, 0)
	order := agent2.PaperOrder{ID: "paper-1", Timestamp: now, Symbol: "ETHUSDT", Side: "BUY", Layer: 1, Price: 100, Quantity: 1, Notional: 100, Status: "OPEN", ExpiresAt: now.Add(time.Hour), InvalidationPrice: 90, Reason: "first"}
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatalf("save open order: %v", err)
	}
	if _, err := db.Exec(`UPDATE paper_orders SET status='FILLED', reason='filled already' WHERE id=?`, order.ID); err != nil {
		t.Fatalf("mark filled: %v", err)
	}
	order.Status = "OPEN"
	order.Price = 95
	order.Reason = "replacement"
	if err := db.SaveOrders([]agent2.PaperOrder{order}); err != nil {
		t.Fatalf("save duplicate order: %v", err)
	}
	var status, reason string
	var price float64
	if err := db.QueryRow(`SELECT status, price, reason FROM paper_orders WHERE id=?`, order.ID).Scan(&status, &price, &reason); err != nil {
		t.Fatalf("query paper order: %v", err)
	}
	if status != "FILLED" || price != 100 || reason != "filled already" {
		t.Fatalf("closed paper order overwritten: status=%s price=%v reason=%s", status, price, reason)
	}
}

func TestManagedLiveOrderReservationLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	desired := liveguard.ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, Source: "test", InvalidationPrice: 90, DecisionReason: "active", ExpiresAt: time.Unix(1700003600, 0)}
	if err := db.ReserveManagedLiveOrder("client-r", desired, "test reserve"); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := db.ReserveManagedLiveOrder("client-r", desired, "duplicate"); err == nil {
		t.Fatal("expected duplicate reservation error")
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Status != live.StatusPlanned || open[0].LayerIndex != 1 || open[0].ExpiresAt != desired.ExpiresAt.Unix() {
		t.Fatalf("bad planned order: %+v", open)
	}
	if err := db.MarkManagedLiveOrderSubmitted("client-r", live.OrderResult{InstID: "ETH-USDT", OrderID: "ord-r", ClientOrderID: "client-r", Submitted: true}); err != nil {
		t.Fatalf("mark submitted: %v", err)
	}
	open, err = db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Status != live.StatusSubmitted || open[0].OrderID != "ord-r" {
		t.Fatalf("bad submitted order: %+v", open)
	}
	if err := db.MarkManagedLiveOrderRejected("client-r", "test reject"); err != nil {
		t.Fatalf("mark rejected: %v", err)
	}
	open, err = db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("rejected order should not be open: %+v", open)
	}
}

func TestOpenLiveOrdersDetailedIncludesCanonicalAndLegacyOpenStatuses(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statuses := []string{live.StatusPlanned, live.StatusSubmitted, live.StatusPartialFill, live.StatusLiveOpen, live.StatusPartiallyFilled, live.StatusFilled, live.StatusCancelled, live.StatusRejected}
	for i, status := range statuses {
		if err := db.SaveManagedLiveOrder(fmt.Sprintf("client-%d", i), fmt.Sprintf("order-%d", i), "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, 0.02, 2, status, live.OrderStatus{LayerIndex: 1}); err != nil {
			t.Fatal(err)
		}
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 5 {
		t.Fatalf("open len=%d want 5: %+v", len(open), open)
	}
}

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
		UpdatedAt:         1700000000,
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
	var updatedAt int64
	var payload string
	if err := db.QueryRow(`SELECT status, updated_at, payload_json FROM live_orders WHERE client_order_id=?`, "client-1").Scan(&savedStatus, &updatedAt, &payload); err != nil {
		t.Fatalf("query saved order: %v", err)
	}
	if savedStatus != live.StatusFilled || updatedAt != 1700000000 || payload == "" {
		t.Fatalf("bad saved status=%q updated_at=%d payload=%q", savedStatus, updatedAt, payload)
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
	if savedStatus != live.StatusCancelled {
		t.Fatalf("status=%s want %s", savedStatus, live.StatusCancelled)
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

func TestManagedLiveOrderStorageRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	meta := live.OrderStatus{LayerIndex: 2, Source: "deterministic_agent2_layer_2", InvalidationPrice: 88, ExpiresAt: 1700003600, DecisionReason: "active layer", LastManagementAction: "placed"}
	if err := db.SaveManagedLiveOrder("client-m", "order-m", "ETH-USDT", "ETHUSDT", "BUY", "limit", 95, 0.02, 1.9, live.StatusLiveOpen, meta); err != nil {
		t.Fatal(err)
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("open len=%d", len(open))
	}
	got := open[0]
	if got.Symbol != "ETHUSDT" || got.LayerIndex != 2 || got.Source != meta.Source || got.InvalidationPrice != 88 || got.DecisionReason != "active layer" || got.LastManagementAction != "placed" {
		t.Fatalf("bad metadata: %+v", got)
	}
}

func TestOpenLiveOrdersHydratesPartialFillSnapshot(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "partial.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveManagedLiveOrder("c-partial", "o1", "BTC-USDT", "BTCUSDT", "SELL", "limit", 50000, .01, 500, live.StatusPartialFill, live.OrderStatus{Source: "HERMES_OPERATOR"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLiveFillSnapshot(live.LiveFillSnapshot{ClientOrderID: "c-partial", OrderID: "o1", InstID: "BTC-USDT", Symbol: "BTCUSDT", Side: "SELL", FilledQuantity: .004, AvgPrice: 50010}); err != nil {
		t.Fatal(err)
	}
	orders, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || math.Abs(orders[0].FilledQuantity-.004) > 1e-12 || math.Abs(orders[0].AvgPrice-50010) > 1e-12 {
		t.Fatalf("partial fill not hydrated: %+v", orders)
	}
}
