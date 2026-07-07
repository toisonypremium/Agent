package paper

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

const (
	StatusOpen        = "OPEN"
	StatusFilled      = "FILLED"
	StatusCancelled   = "CANCELLED"
	StatusExpired     = "EXPIRED"
	StatusInvalidated = "INVALIDATED"
)

type ManagerResult struct {
	GeneratedAt       time.Time           `json:"generated_at"`
	Summary           string              `json:"summary"`
	Checked           int                 `json:"checked"`
	Filled            int                 `json:"filled"`
	Expired           int                 `json:"expired"`
	Cancelled         int                 `json:"cancelled"`
	Invalidated       int                 `json:"invalidated"`
	StillOpen         int                 `json:"still_open"`
	Events            []OrderEvent        `json:"events"`
	OpenOrders        []agent2.PaperOrder `json:"open_orders,omitempty"`
	NoRealOrderPlaced bool                `json:"no_real_order_placed"`
	Notes             []string            `json:"notes,omitempty"`
}

type OrderEvent struct {
	OrderID           string    `json:"order_id"`
	Symbol            string    `json:"symbol"`
	Layer             int       `json:"layer"`
	PreviousStatus    string    `json:"previous_status"`
	NewStatus         string    `json:"new_status"`
	Reason            string    `json:"reason"`
	CandleTime        time.Time `json:"candle_time,omitempty"`
	Price             float64   `json:"price,omitempty"`
	InvalidationPrice float64   `json:"invalidation_price,omitempty"`
}

func ManageOpenOrders(now time.Time, orders []agent2.PaperOrder, candles map[string][]market.Candle, latestPlan agent2.Plan) ManagerResult {
	result := ManagerResult{GeneratedAt: now, Events: []OrderEvent{}, OpenOrders: []agent2.PaperOrder{}, NoRealOrderPlaced: true, Notes: []string{"Paper simulation only; no real order was placed or canceled.", "OHLCV ordering is unknown; same-candle invalidation is treated before fill."}}
	active := activePlanLayers(latestPlan)
	for _, order := range orders {
		if order.Status != StatusOpen {
			continue
		}
		result.Checked++
		symbol := strings.ToUpper(order.Symbol)
		event, closed := candleCloseEvent(order, candles[symbol])
		if !closed && !active[planKey(symbol, order.Layer)] {
			event = OrderEvent{OrderID: order.ID, Symbol: symbol, Layer: order.Layer, PreviousStatus: order.Status, NewStatus: StatusCancelled, Reason: "latest plan no longer has matching ACTIVE_LIMIT layer", Price: order.Price, InvalidationPrice: order.InvalidationPrice}
			closed = true
		}
		if !closed && !order.ExpiresAt.IsZero() && (now.Equal(order.ExpiresAt) || now.After(order.ExpiresAt)) {
			event = OrderEvent{OrderID: order.ID, Symbol: symbol, Layer: order.Layer, PreviousStatus: order.Status, NewStatus: StatusExpired, Reason: "paper order expired", Price: order.Price, InvalidationPrice: order.InvalidationPrice}
			closed = true
		}
		if closed {
			result.Events = append(result.Events, event)
			increment(&result, event.NewStatus)
			continue
		}
		result.StillOpen++
		result.OpenOrders = append(result.OpenOrders, order)
	}
	result.Summary = fmt.Sprintf("paper manager: checked=%d filled=%d expired=%d cancelled=%d invalidated=%d still_open=%d", result.Checked, result.Filled, result.Expired, result.Cancelled, result.Invalidated, result.StillOpen)
	return result
}

func candleCloseEvent(order agent2.PaperOrder, candles []market.Candle) (OrderEvent, bool) {
	for _, candle := range candles {
		if !candle.OpenTime.After(order.Timestamp) {
			continue
		}
		base := OrderEvent{OrderID: order.ID, Symbol: strings.ToUpper(order.Symbol), Layer: order.Layer, PreviousStatus: order.Status, CandleTime: candle.OpenTime, Price: order.Price, InvalidationPrice: order.InvalidationPrice}
		if order.InvalidationPrice > 0 && candle.Low <= order.InvalidationPrice {
			base.NewStatus = StatusInvalidated
			base.Reason = "paper order invalidated before fill on stored candle"
			return base, true
		}
		if strings.EqualFold(order.Side, "BUY") && order.Price > 0 && candle.Low <= order.Price {
			base.NewStatus = StatusFilled
			base.Reason = "paper buy limit filled on stored candle"
			return base, true
		}
	}
	return OrderEvent{}, false
}

func activePlanLayers(plan agent2.Plan) map[string]bool {
	out := map[string]bool{}
	if plan.State != agent2.StateActiveLimit {
		return out
	}
	for _, asset := range plan.Assets {
		if asset.State != agent2.StateActiveLimit {
			continue
		}
		symbol := strings.ToUpper(asset.Symbol)
		for _, layer := range asset.Layers {
			out[planKey(symbol, layer.Index)] = true
		}
	}
	return out
}

func planKey(symbol string, layer int) string {
	return strings.ToUpper(symbol) + "#" + fmt.Sprint(layer)
}

func increment(result *ManagerResult, status string) {
	switch status {
	case StatusFilled:
		result.Filled++
	case StatusExpired:
		result.Expired++
	case StatusCancelled:
		result.Cancelled++
	case StatusInvalidated:
		result.Invalidated++
	}
}

func Markdown(result ManagerResult) string {
	md := fmt.Sprintf("PAPER ORDER MANAGER\n\nGenerated: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary)
	md += fmt.Sprintf("Checked: %d | Filled: %d | Expired: %d | Cancelled: %d | Invalidated: %d | Still open: %d\n", result.Checked, result.Filled, result.Expired, result.Cancelled, result.Invalidated, result.StillOpen)
	if len(result.Events) > 0 {
		md += "\nEvents:\n"
		for _, event := range result.Events {
			when := ""
			if !event.CandleTime.IsZero() {
				when = " candle=" + event.CandleTime.Format("2006-01-02")
			}
			md += fmt.Sprintf("- %s %s layer=%d %s -> %s%s | %s\n", event.OrderID, event.Symbol, event.Layer, event.PreviousStatus, event.NewStatus, when, event.Reason)
		}
	}
	if len(result.OpenOrders) > 0 {
		md += "\nStill open:\n"
		orders := append([]agent2.PaperOrder(nil), result.OpenOrders...)
		sort.SliceStable(orders, func(i, j int) bool {
			if orders[i].Symbol == orders[j].Symbol {
				return orders[i].Layer < orders[j].Layer
			}
			return orders[i].Symbol < orders[j].Symbol
		})
		for _, order := range orders {
			md += fmt.Sprintf("- %s %s layer=%d price=%.8f expires=%s\n", order.ID, order.Symbol, order.Layer, order.Price, order.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
		}
	}
	if len(result.Notes) > 0 {
		md += "\nNotes:\n"
		for _, note := range result.Notes {
			md += "- " + note + "\n"
		}
	}
	md += "\nNo real order was placed or canceled. Paper simulation only.\n"
	return md
}
