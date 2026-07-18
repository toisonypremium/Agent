package liveguard

import (
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
	"testing"
)

func TestBuildAutonomousExitActionsOwnershipAndDedup(t *testing.T) {
	exits := []ExitDecision{{Symbol: "BTCUSDT", Action: ExitTakeProfit, SellPrice: 110, SellQuantity: 2}, {Symbol: "ETHUSDT", Action: ExitPanicSell, SellPrice: 90, SellQuantity: 1}}
	owned := []live.LivePosition{{Symbol: "BTCUSDT", Quantity: 1, AvgEntryPrice: 100}, {Symbol: "ETHUSDT", Quantity: 1, AvgEntryPrice: 100}}
	open := []live.OrderStatus{{Symbol: "ETHUSDT", Side: "SELL", Status: live.StatusSubmitted}}
	got := BuildAutonomousExitActions(exits, owned, open)
	if len(got) != 1 || got[0].Action.Symbol != "BTCUSDT" || got[0].Action.Intent != hermesoperator.IntentReduce {
		t.Fatalf("unexpected %+v", got)
	}
	if got[0].Decision.SellQuantity != 1 || got[0].Action.RequestedNotionalUSDT != 110 {
		t.Fatalf("not ownership capped %+v", got[0])
	}
}
func TestBuildAutonomousExitActionsRejectsUnowned(t *testing.T) {
	got := BuildAutonomousExitActions([]ExitDecision{{Symbol: "BTCUSDT", Action: ExitTrailingStop, SellPrice: 100, SellQuantity: 1}}, nil, nil)
	if len(got) != 0 {
		t.Fatalf("unowned exit generated: %+v", got)
	}
}

func TestBuildAutonomousExitActionsRejectsLossAndWarnings(t *testing.T) {
	owned := []live.LivePosition{{Symbol: "ETHUSDT", Quantity: 2, AvgEntryPrice: 100}}
	for _, ex := range []ExitDecision{
		{Symbol: "ETHUSDT", Action: ExitPanicSell, SellPrice: 70, SellQuantity: 2, PnLPct: -.30},
		{Symbol: "ETHUSDT", Action: ExitHold, SellPrice: 70, SellQuantity: 0, PnLPct: -.30, Warning: true},
	} {
		if got := BuildAutonomousExitActions([]ExitDecision{ex}, owned, nil); len(got) != 0 {
			t.Fatalf("loss exit generated action: %+v", got)
		}
	}
}
