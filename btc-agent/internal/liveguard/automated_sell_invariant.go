package liveguard

import (
	"fmt"
	"math"
	"strings"

	"btc-agent/internal/exchange/live"
)

// AutomatedSellCheck contains the execution-time facts required to authorize
// an automated SELL. All automated exit paths must pass this invariant before
// they create or submit an exchange order.
type AutomatedSellCheck struct {
	Position        live.LivePosition
	Symbol          string
	SellPrice       float64
	SellQuantity    float64
	ReservedSellQty float64
}

// ValidateAutomatedSell fails closed unless the SELL is backed by owned,
// unreserved inventory and cannot realize a loss relative to average entry.
func ValidateAutomatedSell(check AutomatedSellCheck) error {
	positionSymbol := strings.ToUpper(strings.TrimSpace(check.Position.Symbol))
	symbol := strings.ToUpper(strings.TrimSpace(check.Symbol))
	if symbol == "" || positionSymbol == "" || symbol != positionSymbol {
		return fmt.Errorf("automated sell symbol/ownership mismatch: order=%q position=%q", symbol, positionSymbol)
	}
	if !finitePositive(check.Position.Quantity) {
		return fmt.Errorf("automated sell requires positive finite owned position")
	}
	avgEntry := check.Position.AvgEntryPrice
	if !finitePositive(avgEntry) && finitePositive(check.Position.CostBasis) {
		avgEntry = check.Position.CostBasis / check.Position.Quantity
	}
	if !finitePositive(avgEntry) {
		return fmt.Errorf("automated sell requires positive finite average entry or cost basis")
	}
	if !finitePositive(check.SellPrice) || !finitePositive(check.SellQuantity) {
		return fmt.Errorf("automated sell requires positive finite price and quantity")
	}
	if math.IsNaN(check.ReservedSellQty) || math.IsInf(check.ReservedSellQty, 0) || check.ReservedSellQty < 0 {
		return fmt.Errorf("automated sell reserved quantity invalid")
	}
	if check.SellPrice+fillEpsilon < avgEntry {
		return fmt.Errorf("automated loss sale forbidden: sell_price=%.8f below avg_entry=%.8f", check.SellPrice, avgEntry)
	}
	available := check.Position.Quantity - check.ReservedSellQty
	if available <= fillEpsilon || check.SellQuantity > available+fillEpsilon {
		return fmt.Errorf("automated sell exceeds unreserved ownership: owned=%.12f reserved=%.12f sell=%.12f", check.Position.Quantity, check.ReservedSellQty, check.SellQuantity)
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
