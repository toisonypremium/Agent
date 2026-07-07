package paper

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func TestManageOpenOrdersFillsOnlyAfterOrderTimestamp(t *testing.T) {
	now := time.Unix(1700000000, 0)
	order := testOrder(now, 100, 90)
	candles := []market.Candle{
		{Symbol: "ETHUSDT", OpenTime: now, Low: 80},
		{Symbol: "ETHUSDT", OpenTime: now.Add(24 * time.Hour), Low: 99},
	}
	got := ManageOpenOrders(now.Add(2*24*time.Hour), []agent2.PaperOrder{order}, map[string][]market.Candle{"ETHUSDT": candles}, activePlan())
	if got.Filled != 1 || len(got.Events) != 1 || got.Events[0].NewStatus != StatusFilled {
		t.Fatalf("expected fill on next candle: %+v", got)
	}
	if got.Events[0].CandleTime.Equal(now) {
		t.Fatal("same candle must not fill")
	}
}

func TestManageOpenOrdersKeepsOpenWithoutEvent(t *testing.T) {
	now := time.Unix(1700000000, 0)
	order := testOrder(now, 100, 90)
	candles := []market.Candle{{Symbol: "ETHUSDT", OpenTime: now.Add(24 * time.Hour), Low: 101}}
	got := ManageOpenOrders(now.Add(time.Hour), []agent2.PaperOrder{order}, map[string][]market.Candle{"ETHUSDT": candles}, activePlan())
	if got.StillOpen != 1 || len(got.OpenOrders) != 1 || len(got.Events) != 0 {
		t.Fatalf("expected still open: %+v", got)
	}
}

func TestManageOpenOrdersExpires(t *testing.T) {
	now := time.Unix(1700000000, 0)
	order := testOrder(now, 100, 90)
	order.ExpiresAt = now.Add(time.Hour)
	got := ManageOpenOrders(now.Add(2*time.Hour), []agent2.PaperOrder{order}, nil, activePlan())
	if got.Expired != 1 || got.Events[0].NewStatus != StatusExpired {
		t.Fatalf("expected expired: %+v", got)
	}
}

func TestManageOpenOrdersInvalidatesBeforeFill(t *testing.T) {
	now := time.Unix(1700000000, 0)
	order := testOrder(now, 100, 90)
	candles := []market.Candle{{Symbol: "ETHUSDT", OpenTime: now.Add(24 * time.Hour), Low: 89}}
	got := ManageOpenOrders(now.Add(2*24*time.Hour), []agent2.PaperOrder{order}, map[string][]market.Candle{"ETHUSDT": candles}, activePlan())
	if got.Invalidated != 1 || got.Filled != 0 || got.Events[0].NewStatus != StatusInvalidated {
		t.Fatalf("expected invalidation before fill: %+v", got)
	}
}

func TestManageOpenOrdersCancelsWhenPlanNotActive(t *testing.T) {
	now := time.Unix(1700000000, 0)
	order := testOrder(now, 100, 90)
	got := ManageOpenOrders(now.Add(time.Hour), []agent2.PaperOrder{order}, nil, agent2.Plan{State: agent2.StateWatch})
	if got.Cancelled != 1 || got.Events[0].NewStatus != StatusCancelled || !strings.Contains(got.Events[0].Reason, "latest plan") {
		t.Fatalf("expected inactive plan cancel: %+v", got)
	}
}

func TestMarkdownIncludesSafetyNote(t *testing.T) {
	now := time.Unix(1700000000, 0)
	md := Markdown(ManagerResult{GeneratedAt: now, Summary: "paper manager: checked=0", NoRealOrderPlaced: true})
	for _, want := range []string{"PAPER ORDER MANAGER", "No real order was placed or canceled", "Paper simulation only"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func testOrder(now time.Time, price, invalidation float64) agent2.PaperOrder {
	return agent2.PaperOrder{ID: "order-1", Timestamp: now, Symbol: "ETHUSDT", Side: "BUY", Layer: 1, Price: price, Quantity: 1, Notional: price, InvalidationPrice: invalidation, Status: StatusOpen, ExpiresAt: now.Add(48 * time.Hour), Reason: "test"}
}

func activePlan() agent2.Plan {
	return agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1}}}}}
}
