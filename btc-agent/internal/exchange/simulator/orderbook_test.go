package simulator

import (
	"context"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestOrderbookSimulationProcessCandle(t *testing.T) {
	fake := NewFakeOKX()
	fake.SetFilter("RENDER-USDT", live.InstrumentFilter{InstID: "RENDER-USDT", MinSize: 0.1, MinNotional: 1})
	ctx := context.Background()
	_, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: "RENDER-USDT", Side: "buy", Price: 5, Quantity: 10, ClientOrderID: "render1"})
	if err != nil {
		t.Fatal(err)
	}

	sim := &FillSimulation{OKX: fake}
	if err := sim.ProcessCandle(ctx, "RENDER-USDT", 6, 5.5, 5.8, 1000); err != nil {
		t.Fatal(err)
	}
	order, err := fake.GetOrder(ctx, "RENDER-USDT", "", "render1")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusSubmitted {
		t.Fatalf("expected untouched, got %s", order.Status)
	}

	if err := sim.ProcessCandle(ctx, "RENDER-USDT", 6, 4, 4.5, 1000); err != nil {
		t.Fatal(err)
	}
	order, err = fake.GetOrder(ctx, "RENDER-USDT", "", "render1")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusFilled || order.FilledQuantity != 10 {
		t.Fatalf("expected filled 10, got %+v", order)
	}
}

func TestOrderbookSimulationExactTouchPartial(t *testing.T) {
	fake := NewFakeOKX()
	fake.SetFilter("ETH-USDT", live.InstrumentFilter{InstID: "ETH-USDT", MinSize: 0.01, MinNotional: 1})
	ctx := context.Background()
	_, err := fake.PlaceSpotLimitOrder(ctx, live.LimitOrderRequest{InstID: "ETH-USDT", Side: "buy", Price: 100, Quantity: 2, ClientOrderID: "eth-touch"})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&FillSimulation{OKX: fake}).ProcessCandle(ctx, "ETH-USDT", 105, 100, 101, 1000); err != nil {
		t.Fatal(err)
	}
	order, err := fake.GetOrder(ctx, "ETH-USDT", "", "eth-touch")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusPartialFill || order.FilledQuantity != 1 {
		t.Fatalf("expected conservative partial touch fill, got %+v", order)
	}
}
