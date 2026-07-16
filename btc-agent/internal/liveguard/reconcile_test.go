package liveguard

import (
	"context"
	"errors"
	"testing"

	"btc-agent/internal/exchange/live"
)

type mockReader struct {
	status func(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error)
}

func (m *mockReader) OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
	return m.status(ctx, instID, orderID, clientOrderID)
}

func (m *mockReader) PendingOrders(ctx context.Context, instID string) ([]live.OrderStatus, error) {
	return nil, nil
}

func TestReconcileOrders(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		res := ReconcileOrders(context.Background(), &mockReader{}, nil)
		if res.Checked != 0 {
			t.Errorf("expected 0 checked, got %d", res.Checked)
		}
	})

	t.Run("order filled", func(t *testing.T) {
		reader := &mockReader{
			status: func(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
				return live.OrderStatus{
					InstID:        instID,
					OrderID:       orderID,
					ClientOrderID: clientOrderID,
					Status:        live.StatusFilled,
				}, nil
			},
		}

		open := []live.OrderStatus{
			{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "c1", Status: live.StatusLiveOpen},
		}

		res := ReconcileOrders(context.Background(), reader, open)
		if res.Checked != 1 || res.Updated != 1 || res.Unknown != 0 {
			t.Errorf("unexpected results: %+v", res)
		}
		if res.Orders[0].Status != live.StatusFilled {
			t.Errorf("expected status FILLED, got %s", res.Orders[0].Status)
		}
	})

	t.Run("remote response keeps local identity", func(t *testing.T) {
		reader := &mockReader{
			status: func(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
				return live.OrderStatus{Status: live.StatusFilled}, nil
			},
		}

		open := []live.OrderStatus{
			{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "c1", Status: live.StatusLiveOpen},
		}

		res := ReconcileOrders(context.Background(), reader, open)
		if res.Orders[0].InstID != "ETH-USDT" || res.Orders[0].OrderID != "123" || res.Orders[0].ClientOrderID != "c1" {
			t.Fatalf("remote identity not preserved: %+v", res.Orders[0])
		}
	})

	t.Run("nil reader marks unknown", func(t *testing.T) {
		open := []live.OrderStatus{
			{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "c1", Status: live.StatusLiveOpen},
		}

		res := ReconcileOrders(context.Background(), nil, open)
		if res.Checked != 1 || res.Updated != 0 || res.Unknown != 1 {
			t.Errorf("unexpected results: %+v", res)
		}
		if res.Orders[0].Status != live.StatusUnknownNeedsManualCheck {
			t.Errorf("expected status UNKNOWN, got %s", res.Orders[0].Status)
		}
	})

	t.Run("reader error", func(t *testing.T) {
		reader := &mockReader{
			status: func(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
				return live.OrderStatus{}, errors.New("network error")
			},
		}

		open := []live.OrderStatus{
			{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "c1", Status: live.StatusLiveOpen},
		}

		res := ReconcileOrders(context.Background(), reader, open)
		if res.Checked != 1 || res.Updated != 0 || res.Unknown != 1 {
			t.Errorf("unexpected results: %+v", res)
		}
		if res.Orders[0].Status != live.StatusUnknownNeedsManualCheck {
			t.Errorf("expected status UNKNOWN, got %s", res.Orders[0].Status)
		}
	})
}

func TestReconcileSafetyBlocksUnknownAndBadFill(t *testing.T) {
	res := ReconcileResult{Checked: 2, Unknown: 1, Orders: []live.OrderStatus{
		{ClientOrderID: "c1", OrderID: "o1", Status: live.StatusUnknownNeedsManualCheck},
		{ClientOrderID: "c2", OrderID: "o2", Status: live.StatusFilled},
	}}
	got := ReconcileSafety(res)
	if got.Status != ReconcileBlock || len(got.Blockers) == 0 {
		t.Fatalf("expected reconcile block: %+v", got)
	}
}

func TestReconcileSafetyClean(t *testing.T) {
	got := ReconcileSafety(ReconcileResult{Checked: 1, Orders: []live.OrderStatus{{ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen}}})
	if got.Status != ReconcileClean || len(got.Blockers) != 0 {
		t.Fatalf("expected clean safety: %+v", got)
	}
}

func TestReconcileSafetyHandlesCanonicalPartialFill(t *testing.T) {
	bad := ReconcileSafety(ReconcileResult{Checked: 1, Orders: []live.OrderStatus{{ClientOrderID: "c1", OrderID: "o1", Status: live.StatusPartialFill}}})
	if bad.Status != ReconcileBlock || len(bad.Blockers) == 0 {
		t.Fatalf("partial fill missing fill details should block: %+v", bad)
	}
	good := ReconcileSafety(ReconcileResult{Checked: 1, Orders: []live.OrderStatus{{ClientOrderID: "c1", OrderID: "o1", Status: live.StatusPartialFill, AccumulatedFillSz: 0.01, AvgPrice: 100}}})
	if good.Status != ReconcileClean || len(good.Blockers) != 0 {
		t.Fatalf("partial fill with details should be clean: %+v", good)
	}
}

func TestApplyHaltedReconcileInvariantClean(t *testing.T) {
	got := ApplyHaltedReconcileInvariant(ReconcileOrders(context.Background(), nil, nil), nil, true)
	if got.Safety.Status != ReconcileClean || !got.Safety.OperatorHalted || got.Safety.OpenAfterReconcile != 0 || got.Safety.UnknownPositions != 0 {
		t.Fatalf("expected clean halted invariant: %+v", got.Safety)
	}
}

func TestApplyHaltedReconcileInvariantBlocksResidualOpenOrder(t *testing.T) {
	result := ReconcileResult{Checked: 1, Orders: []live.OrderStatus{{ClientOrderID: "c1", OrderID: "o1", Status: live.StatusLiveOpen}}}
	result.Safety = ReconcileSafety(result)
	got := ApplyHaltedReconcileInvariant(result, nil, true)
	if got.Safety.Status != ReconcileBlock || got.Safety.OpenAfterReconcile != 1 {
		t.Fatalf("expected residual open order to block: %+v", got.Safety)
	}
}

func TestApplyHaltedReconcileInvariantBlocksMalformedPositivePosition(t *testing.T) {
	got := ApplyHaltedReconcileInvariant(ReconcileOrders(context.Background(), nil, nil), []live.LivePosition{{Symbol: "BTCUSDT", Quantity: 0.01}}, true)
	if got.Safety.Status != ReconcileBlock || got.Safety.UnknownPositions != 1 {
		t.Fatalf("expected malformed positive position to block: %+v", got.Safety)
	}
	closed := ApplyHaltedReconcileInvariant(ReconcileOrders(context.Background(), nil, nil), []live.LivePosition{{Symbol: "BTCUSDT", Quantity: 0}}, true)
	if closed.Safety.Status != ReconcileClean || closed.Safety.UnknownPositions != 0 {
		t.Fatalf("closed ledger row must not block: %+v", closed.Safety)
	}
}
