package simulator

import (
	"context"
	"strings"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestFakeOKXPlaceSpotLimitOrderConstraints(t *testing.T) {
	fake := NewFakeOKX()
	fake.SetBalance("USDT", 1000)
	fake.SetFilter("BTC-USDT", live.InstrumentFilter{InstID: "BTC-USDT", MinNotional: 10, MinSize: 0.0001, TickSize: 0.1, StepSize: 0.0001})

	ctx := context.Background()
	res, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: "BTC-USDT", Side: "buy", Price: 60000, Quantity: 0.001, ClientOrderID: "cl1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.OrderID == "" || !res.Submitted {
		t.Fatalf("expected order id and submitted, got %+v", res)
	}

	cases := []struct {
		name string
		req  live.LimitOrderRequest
		want string
	}{
		{"min size", live.LimitOrderRequest{InstID: "BTC-USDT", Side: "buy", Price: 60000, Quantity: 0.00005, ClientOrderID: "cl2"}, "min size"},
		{"min notional", live.LimitOrderRequest{InstID: "BTC-USDT", Side: "buy", Price: 10000, Quantity: 0.0005, ClientOrderID: "cl3"}, "min notional"},
		{"tick", live.LimitOrderRequest{InstID: "BTC-USDT", Side: "buy", Price: 60000.05, Quantity: 0.001, ClientOrderID: "cl4"}, "tick size"},
		{"step", live.LimitOrderRequest{InstID: "BTC-USDT", Side: "buy", Price: 60000, Quantity: 0.00105, ClientOrderID: "cl5"}, "step size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fake.PlaceSpotLimitOrder(ctx, tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestFakeOKXPartialAndFullFill(t *testing.T) {
	fake := NewFakeOKX()
	fake.SetFilter("ETH-USDT", live.InstrumentFilter{InstID: "ETH-USDT", MinSize: 0.01, MinNotional: 2})
	ctx := context.Background()
	_, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: "ETH-USDT", Side: "buy", Price: 3000, Quantity: 1, ClientOrderID: "cl-eth-1"})
	if err != nil {
		t.Fatal(err)
	}

	if err := fake.SimFill("cl-eth-1", 0.4, 3000); err != nil {
		t.Fatal(err)
	}
	order, err := fake.GetOrder(ctx, "ETH-USDT", "", "cl-eth-1")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusPartialFill || order.FilledQuantity != 0.4 {
		t.Fatalf("expected partial 0.4, got %+v", order)
	}

	if err := fake.SimFill("cl-eth-1", 0.6, 3000); err != nil {
		t.Fatal(err)
	}
	order, err = fake.GetOrder(ctx, "ETH-USDT", "", "cl-eth-1")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusFilled || order.FilledQuantity != 1 {
		t.Fatalf("expected filled 1.0, got %+v", order)
	}
	if order.Fee <= 0 || order.FeeCurrency != "USDT" {
		t.Fatalf("expected maker fee, got %+v", order)
	}
}

func TestFakeOKXCancelReleasesReservedBalance(t *testing.T) {
	fake := NewFakeOKX()
	fake.SetBalance("USDT", 100)
	fake.SetFilter("SOL-USDT", live.InstrumentFilter{InstID: "SOL-USDT", MinSize: 0.1, MinNotional: 5})
	ctx := context.Background()
	_, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: "SOL-USDT", Side: "buy", Price: 10, Quantity: 5, ClientOrderID: "sol-1"})
	if err != nil {
		t.Fatal(err)
	}
	balances, _ := fake.AccountBalance(ctx, "USDT")
	if balances[0].Free != 50 {
		t.Fatalf("expected reserved balance 50, got %.2f", balances[0].Free)
	}
	_, err = fake.CancelOrder(ctx, live.CancelOrderRequest{InstID: "SOL-USDT", ClientOrderID: "sol-1"})
	if err != nil {
		t.Fatal(err)
	}
	balances, _ = fake.AccountBalance(ctx, "USDT")
	if balances[0].Free != 100 {
		t.Fatalf("expected released balance 100, got %.2f", balances[0].Free)
	}
}
