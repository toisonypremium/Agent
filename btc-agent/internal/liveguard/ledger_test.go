package liveguard

import (
	"math"
	"testing"
	"time"

	"btc-agent/internal/exchange/live"
)

func TestBuildPositionEvent(t *testing.T) {
	now := time.Unix(1700000000, 0)
	base := live.OrderStatus{
		InstID:            "ETH-USDT",
		OrderID:           "order-1",
		ClientOrderID:     "client-1",
		Status:            live.StatusPartiallyFilled,
		Side:              "buy",
		Price:             2000,
		AvgPrice:          1999,
		AccumulatedFillSz: 0.01,
		Fee:               -0.001,
		FeeCurrency:       "USDT",
		UpdatedAt:         1700000000000,
	}

	t.Run("first partial fill creates event", func(t *testing.T) {
		event, ok, err := BuildPositionEvent(live.LiveFillSnapshot{}, base, now)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected event")
		}
		if event.Symbol != "ETHUSDT" || event.Side != "BUY" || !nearLedger(event.DeltaQuantity, 0.01) || event.FillPrice != 1999 || !nearLedger(event.NotionalDelta, 19.99) || !nearLedger(event.FeeDelta, -0.001) || event.FeeCurrency != "USDT" {
			t.Fatalf("bad event: %+v", event)
		}
	})

	t.Run("same cumulative fill is idempotent", func(t *testing.T) {
		prev := live.LiveFillSnapshot{FilledQuantity: 0.01, Fee: -0.001, FeeCurrency: "USDT"}
		_, ok, err := BuildPositionEvent(prev, base, now)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("expected no-op")
		}
	})

	t.Run("partial to filled emits only incremental delta", func(t *testing.T) {
		prev := live.LiveFillSnapshot{FilledQuantity: 0.01, Fee: -0.001, FeeCurrency: "USDT"}
		filled := base
		filled.Status = live.StatusFilled
		filled.AccumulatedFillSz = 0.025
		filled.Fee = -0.0025
		event, ok, err := BuildPositionEvent(prev, filled, now)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected event")
		}
		if !nearLedger(event.DeltaQuantity, 0.015) || !nearLedger(event.FeeDelta, -0.0015) {
			t.Fatalf("bad incremental event: %+v", event)
		}
	})

	t.Run("lower remote cumulative fill errors", func(t *testing.T) {
		prev := live.LiveFillSnapshot{FilledQuantity: 0.02}
		_, _, err := BuildPositionEvent(prev, base, now)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown status no-ops", func(t *testing.T) {
		unknown := base
		unknown.Status = live.StatusUnknownNeedsManualCheck
		_, ok, err := BuildPositionEvent(live.LiveFillSnapshot{}, unknown, now)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("expected no-op")
		}
	})

	t.Run("missing fill price errors", func(t *testing.T) {
		bad := base
		bad.Price = 0
		bad.AvgPrice = 0
		_, _, err := BuildPositionEvent(live.LiveFillSnapshot{}, bad, now)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFillSnapshotFromStatus(t *testing.T) {
	status := live.OrderStatus{InstID: "ETH-USDT", OrderID: "o1", ClientOrderID: "c1", Side: "buy", AccumulatedFillSz: 0.1, AvgPrice: 2000, Fee: -0.01, FeeCurrency: "usdt", UpdatedAt: 123}
	got := FillSnapshotFromStatus(status)
	if got.Symbol != "ETHUSDT" || got.Side != "BUY" || got.FilledQuantity != 0.1 || got.AvgPrice != 2000 || got.FeeCurrency != "USDT" || got.UpdatedAt != 123 {
		t.Fatalf("bad snapshot: %+v", got)
	}
}

func nearLedger(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
