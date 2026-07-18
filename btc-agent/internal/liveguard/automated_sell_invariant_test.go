package liveguard

import (
	"math"
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestValidateAutomatedSellInvariantMatrix(t *testing.T) {
	position := live.LivePosition{Symbol: "BTCUSDT", Quantity: 2, AvgEntryPrice: 100}
	cases := []struct {
		name    string
		mutate  func(*AutomatedSellCheck)
		wantErr bool
	}{
		{name: "at cost basis allowed"},
		{name: "above cost basis allowed", mutate: func(c *AutomatedSellCheck) { c.SellPrice = 110 }},
		{name: "below cost basis blocked", mutate: func(c *AutomatedSellCheck) { c.SellPrice = 99.999 }},
		{name: "exact residual allowed", mutate: func(c *AutomatedSellCheck) { c.ReservedSellQty = 1; c.SellQuantity = 1 }},
		{name: "over residual blocked", mutate: func(c *AutomatedSellCheck) { c.ReservedSellQty = 1; c.SellQuantity = 1.001 }},
		{name: "fully reserved blocked", mutate: func(c *AutomatedSellCheck) { c.ReservedSellQty = 2; c.SellQuantity = .1 }},
		{name: "over owned blocked", mutate: func(c *AutomatedSellCheck) { c.SellQuantity = 2.001 }},
		{name: "symbol mismatch blocked", mutate: func(c *AutomatedSellCheck) { c.Symbol = "ETHUSDT" }},
		{name: "zero average blocked", mutate: func(c *AutomatedSellCheck) { c.Position.AvgEntryPrice = 0 }},
		{name: "nan price blocked", mutate: func(c *AutomatedSellCheck) { c.SellPrice = math.NaN() }},
		{name: "infinite quantity blocked", mutate: func(c *AutomatedSellCheck) { c.SellQuantity = math.Inf(1) }},
		{name: "negative reservation blocked", mutate: func(c *AutomatedSellCheck) { c.ReservedSellQty = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := AutomatedSellCheck{Position: position, Symbol: "BTCUSDT", SellPrice: 100, SellQuantity: 1}
			if tc.mutate != nil {
				tc.mutate(&check)
			}
			err := ValidateAutomatedSell(check)
			if (err != nil) != tc.wantErr {
				// Cases with blocked in their name are intentionally errors.
				blocked := len(tc.name) >= 7 && tc.name[len(tc.name)-7:] == "blocked"
				if (err != nil) != blocked {
					t.Fatalf("err=%v blocked=%v", err, blocked)
				}
			}
		})
	}
}

func TestTimeStopBoundaryNeverSellsBelowCost(t *testing.T) {
	cfg := testExitCfg()
	cfg.Exit.TimeStopDays = 1
	cfg.Exit.TakeProfitPct = 10
	cfg.Exit.TrailingActivatePct = 10
	position := testPos("BTCUSDT", 1, 100, 2)
	for _, tc := range []struct {
		name  string
		price float64
		want  ExitAction
	}{
		{name: "below by epsilon", price: 99.999999, want: ExitHold},
		{name: "at cost", price: 100, want: ExitTimeStop},
		{name: "above by epsilon", price: 100.000001, want: ExitTimeStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateExits(cfg, []live.LivePosition{position}, map[string]float64{"BTCUSDT": tc.price}, NewPeakTracker())
			if len(got) != 1 || got[0].Action != tc.want {
				t.Fatalf("got=%+v want=%s", got, tc.want)
			}
		})
	}
}
