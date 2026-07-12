package liveguard

import (
	"context"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/exchange/simulator"
)

func TestManageLiveOrdersWithFakeOKXSubmitAndFill(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	fake := simulator.NewFakeOKX()
	fake.SetBalance("USDT", 1000)
	fake.SetFilter("ETH-USDT", live.InstrumentFilter{InstID: "ETH-USDT", MinNotional: 1, MinSize: 0.0001})
	fake.SetFilter("SOL-USDT", live.InstrumentFilter{InstID: "SOL-USDT", MinNotional: 1, MinSize: 0.0001})

	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, fake, fake, fakeHaltReader{halted: false})
	if got.Status != ManagedCycleCompleted || len(got.Placed) != 4 {
		t.Fatalf("expected 4 simulated submits, got %+v", got)
	}
	clientID := got.Placed[0].PlaceResult.ClientOrderID
	if err := fake.SimFill(clientID, got.Placed[0].Desired.Quantity/2, got.Placed[0].Desired.Price); err != nil {
		t.Fatal(err)
	}
	order, err := fake.GetOrder(context.Background(), got.Placed[0].Desired.InstID, "", clientID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusPartialFill {
		t.Fatalf("expected partial fill, got %+v", order)
	}
	if err := fake.SimFill(clientID, got.Placed[0].Desired.Quantity, got.Placed[0].Desired.Price); err != nil {
		t.Fatal(err)
	}
	order, err = fake.GetOrder(context.Background(), got.Placed[0].Desired.InstID, "", clientID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusFilled || order.Fee <= 0 {
		t.Fatalf("expected full fill with fee, got %+v", order)
	}
}

func TestManageLiveOrdersWithFakeOKXRejectsByFilter(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	fake := simulator.NewFakeOKX()
	fake.SetBalance("USDT", 1000)
	fake.SetFilter("ETH-USDT", live.InstrumentFilter{InstID: "ETH-USDT", MinNotional: 100, MinSize: 0.0001})
	fake.SetFilter("SOL-USDT", live.InstrumentFilter{InstID: "SOL-USDT", MinNotional: 1, MinSize: 0.0001})

	got := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, fake, fake, fakeHaltReader{halted: false})
	if got.Status != ManagedCyclePartial || len(got.Blocked) == 0 {
		t.Fatalf("expected simulator reject to block cycle, got %+v", got)
	}
	found := false
	for _, blocked := range got.Blocked {
		if blocked.Symbol == "ETHUSDT" && blocked.Error != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing ETH rejection: %+v", got.Blocked)
	}
}

func TestManageLiveOrdersWithFakeOKXCancelsInactivePlan(t *testing.T) {
	cfg := managedConfig()
	plan := managedPlan()
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}, "SOLUSDT": {Score: 75, Grade: "B"}})
	fake := simulator.NewFakeOKX()
	fake.SetBalance("USDT", 1000)
	fake.SetFilter("ETH-USDT", live.InstrumentFilter{InstID: "ETH-USDT", MinNotional: 1, MinSize: 0.0001})
	fake.SetFilter("SOL-USDT", live.InstrumentFilter{InstID: "SOL-USDT", MinNotional: 1, MinSize: 0.0001})

	first := manageLiveOrdersConfirmed(context.Background(), cfg, plan, nil, nil, nil, fake, fake, fakeHaltReader{halted: false})
	if first.Status != ManagedCycleCompleted || len(first.Placed) == 0 {
		t.Fatalf("expected initial submits, got %+v", first)
	}
	open := []live.OrderStatus{{InstID: first.Placed[0].Desired.InstID, Symbol: first.Placed[0].Desired.Symbol, ClientOrderID: first.Placed[0].PlaceResult.ClientOrderID, OrderID: first.Placed[0].PlaceResult.OrderID, Status: live.StatusSubmitted, Price: first.Placed[0].Desired.Price, Quantity: first.Placed[0].Desired.Quantity, Notional: first.Placed[0].Desired.Notional, LayerIndex: first.Placed[0].Desired.LayerIndex}}
	inactive := plan
	inactive.State = agent2.StateWatch
	got := manageLiveOrdersConfirmed(context.Background(), cfg, inactive, open, nil, nil, fake, fake, fakeHaltReader{halted: false})
	if got.Status != ManagedCycleCompleted || len(got.Canceled) != 1 {
		t.Fatalf("expected one simulator cancel, got %+v", got)
	}
	order, err := fake.GetOrder(context.Background(), open[0].InstID, "", open[0].ClientOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != live.StatusCancelled {
		t.Fatalf("expected canceled fake order, got %+v", order)
	}
}
