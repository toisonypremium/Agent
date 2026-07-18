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
	ownedQty := map[string]float64{}
	for _, p := range owned {
		if p.Quantity > 0 {
			ownedQty[strings.ToUpper(p.Symbol)] = p.Quantity
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
		if ex.Warning || ex.Action == ExitHold || ex.PnLPct < 0 || qty <= 0 || ex.SellPrice <= 0 || ownedQty[symbol] <= 0 || openSell[symbol] {
			continue
		}
		if qty > ownedQty[symbol] {
			qty = ownedQty[symbol]
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
