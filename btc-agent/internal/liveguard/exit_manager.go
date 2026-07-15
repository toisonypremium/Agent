package liveguard

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

// ExitAction defines what action bot should take on a position.
type ExitAction string

const (
	ExitHold          ExitAction = "HOLD"
	ExitTakeProfit    ExitAction = "TAKE_PROFIT"
	ExitTrailingStop  ExitAction = "TRAILING_STOP"
	ExitTimeStop      ExitAction = "TIME_STOP"
	ExitPanicSell     ExitAction = "PANIC_SELL"
)

// ExitDecision is the output of EvaluateExits for a single position.
type ExitDecision struct {
	Symbol        string     `json:"symbol"`
	Action        ExitAction `json:"action"`
	SellPrice     float64    `json:"sell_price"`   // Limit price to place
	SellQuantity  float64    `json:"sell_quantity"` // Qty to sell (partial or full)
	PnLPct        float64    `json:"pnl_pct"`
	Reason        string     `json:"reason"`
	GeneratedAt   time.Time  `json:"generated_at"`
}

// PeakTracker holds the highest price seen since a position was opened.
// It is maintained in-memory by the supervisor cycle.
type PeakTracker struct {
	PeakBySymbol map[string]float64
	TrailActive  map[string]bool
}

// NewPeakTracker creates an empty tracker.
func NewPeakTracker() *PeakTracker {
	return &PeakTracker{
		PeakBySymbol: map[string]float64{},
		TrailActive:  map[string]bool{},
	}
}

// Update updates the peak price for a symbol and returns whether trailing stop is active.
func (pt *PeakTracker) Update(symbol string, currentPrice, activatePct, avgEntry float64) {
	sym := strings.ToUpper(symbol)
	if currentPrice > pt.PeakBySymbol[sym] {
		pt.PeakBySymbol[sym] = currentPrice
	}
	if avgEntry > 0 && activatePct > 0 {
		pnl := (currentPrice - avgEntry) / avgEntry
		if pnl >= activatePct {
			pt.TrailActive[sym] = true
		}
	}
}

// EvaluateExits evaluates exit conditions for all live positions.
// Returns a list of ExitDecision — caller places SELL limit orders for non-HOLD decisions.
func EvaluateExits(
	cfg config.Config,
	positions []live.LivePosition,
	currentPrices map[string]float64,
	peakTracker *PeakTracker,
) []ExitDecision {
	if !cfg.Exit.Enabled {
		return nil
	}
	if peakTracker == nil {
		peakTracker = NewPeakTracker()
	}

	var decisions []ExitDecision
	now := time.Now().UTC()

	for _, pos := range positions {
		if pos.Quantity <= 0 || pos.AvgEntryPrice <= 0 {
			continue
		}
		sym := strings.ToUpper(pos.Symbol)
		currentPrice, hasCurrent := currentPrices[sym]
		if !hasCurrent || currentPrice <= 0 {
			continue
		}

		// Update peak tracker
		peakTracker.Update(sym, currentPrice, cfg.Exit.TrailingActivatePct, pos.AvgEntryPrice)

		pnlPct := (currentPrice - pos.AvgEntryPrice) / pos.AvgEntryPrice
		peak := peakTracker.PeakBySymbol[sym]
		trailActive := peakTracker.TrailActive[sym]

		decision := ExitDecision{
			Symbol:      sym,
			Action:      ExitHold,
			SellPrice:   currentPrice,
			SellQuantity: pos.Quantity,
			PnLPct:      finiteFloat(pnlPct),
			GeneratedAt: now,
		}

		switch {
		// 1. Take-profit: reached target gain → partial sell
		case cfg.Exit.TakeProfitPct > 0 && pnlPct >= cfg.Exit.TakeProfitPct:
			sellQty := pos.Quantity
			if cfg.Exit.PartialExitPct > 0 && cfg.Exit.PartialExitPct < 1.0 {
				sellQty = pos.Quantity * cfg.Exit.PartialExitPct
			}
			decision.Action = ExitTakeProfit
			decision.SellQuantity = finiteFloat(sellQty)
			decision.Reason = fmt.Sprintf("take-profit: pnl=%.2f%% >= target=%.2f%%; selling %.0f%% of position",
				pnlPct*100, cfg.Exit.TakeProfitPct*100, cfg.Exit.PartialExitPct*100)

		// 2. Trailing stop triggered: price fell from peak by trailing distance
		case trailActive && cfg.Exit.TrailingDistancePct > 0 && peak > 0:
			trailStop := peak * (1 - cfg.Exit.TrailingDistancePct)
			if currentPrice <= trailStop {
				decision.Action = ExitTrailingStop
				decision.SellQuantity = pos.Quantity
				decision.Reason = fmt.Sprintf("trailing stop: price=%.4f <= trail_stop=%.4f (peak=%.4f, dist=%.1f%%)",
					currentPrice, trailStop, peak, cfg.Exit.TrailingDistancePct*100)
			}

		// 3. Time stop: position too old and PnL not recovering
		case cfg.Exit.TimeStopDays > 0 && pos.OpenedAt > 0 && time.Unix(pos.OpenedAt, 0).Before(now.AddDate(0, 0, -cfg.Exit.TimeStopDays)):
			// Don't time-stop if deep in loss (let it recover) unless explicitly below min PnL floor
			if cfg.Exit.MinPnLForTimeStop == 0 || pnlPct >= cfg.Exit.MinPnLForTimeStop {
				decision.Action = ExitTimeStop
				decision.SellQuantity = pos.Quantity
				decision.Reason = fmt.Sprintf("time-stop: position age > %d days, pnl=%.2f%%",
					cfg.Exit.TimeStopDays, pnlPct*100)
			}
		}

		if decision.Action != ExitHold {
			log.Printf("[ExitManager] %s → %s: %s", sym, decision.Action, decision.Reason)
		}
		decisions = append(decisions, decision)
	}
	return decisions
}

// PlaceSellLimitOrder places a sell limit order for an exit decision.
func PlaceSellLimitOrder(ctx context.Context, decision ExitDecision, placer OrderPlacer) (live.OrderResult, error) {
	if decision.SellQuantity <= 0 || decision.SellPrice <= 0 {
		return live.OrderResult{}, fmt.Errorf("invalid exit decision: qty=%.6f price=%.4f", decision.SellQuantity, decision.SellPrice)
	}
	instID := live.OKXInstID(decision.Symbol)
	clOrdID := fmt.Sprintf("exit-%s-%d", strings.ToLower(decision.Symbol), time.Now().UnixMilli()%1e9)
	req := live.LimitOrderRequest{
		InstID:        instID,
		Side:          "sell",
		Price:         math.Round(decision.SellPrice*1e4) / 1e4,
		Quantity:      decision.SellQuantity,
		PostOnly:      false, // Use regular limit for sell exits — fills faster
		ClientOrderID: clOrdID,
	}
	return placer.PlaceSpotLimitOrder(ctx, req)
}
