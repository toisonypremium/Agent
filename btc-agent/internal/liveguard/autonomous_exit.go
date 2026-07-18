package liveguard

import (
	"strings"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
)

// AutonomousExitAction is a deterministic, ownership-checked conversion from
// an exit signal to a Hermes reducing intent. It does not call the exchange.
type AutonomousExitAction struct {
	Decision ExitDecision
	Action   hermesoperator.Action
}

func BuildAutonomousExitActions(exits []ExitDecision, owned []live.LivePosition, open []live.OrderStatus) []AutonomousExitAction {
	ownedBySymbol := map[string]live.LivePosition{}
	for _, p := range owned {
		if p.Quantity > 0 {
			ownedBySymbol[strings.ToUpper(p.Symbol)] = p
		}
	}
	openSell := map[string]bool{}
	for _, o := range open {
		symbol := strings.ToUpper(firstNonEmptyString(o.Symbol, live.InternalSymbol(o.InstID)))
		if strings.EqualFold(o.Side, "SELL") && live.IsOpenStatus(o.Status) {
			openSell[symbol] = true
		}
	}
	out := []AutonomousExitAction{}
	for _, ex := range exits {
		symbol := strings.ToUpper(ex.Symbol)
		qty := ex.SellQuantity
		// Warning-only and loss-making decisions have no execution authority.
		if ex.Warning || ex.Action == ExitHold || ex.PnLPct < 0 || qty <= 0 || ex.SellPrice <= 0 || ownedBySymbol[symbol].Quantity <= 0 || openSell[symbol] {
			continue
		}
		if qty > ownedBySymbol[symbol].Quantity {
			qty = ownedBySymbol[symbol].Quantity
		}
		if err := ValidateAutomatedSell(AutomatedSellCheck{Position: ownedBySymbol[symbol], Symbol: symbol, SellPrice: ex.SellPrice, SellQuantity: qty}); err != nil {
			continue
		}
		intent := hermesoperator.IntentExitLimit
		if ex.Action == ExitTakeProfit {
			intent = hermesoperator.IntentReduce
		}
		action := hermesoperator.Action{Symbol: symbol, Intent: intent, Confidence: 1, EntryPrice: ex.SellPrice, RequestedNotionalUSDT: ex.SellPrice * qty, ReasonCodes: []string{"deterministic_exit", strings.ToLower(string(ex.Action))}}
		ex.SellQuantity = qty
		out = append(out, AutonomousExitAction{Decision: ex, Action: action})
	}
	return out
}
