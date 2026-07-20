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
	ExitHold         ExitAction = "HOLD"
	ExitTakeProfit   ExitAction = "TAKE_PROFIT"
	ExitTrailingStop ExitAction = "TRAILING_STOP"
	ExitTimeStop     ExitAction = "TIME_STOP"
	ExitPanicSell    ExitAction = "PANIC_SELL"
)

// ExitDecision is the output of EvaluateExits for a single position.
type ExitDecision struct {
	Symbol       string     `json:"symbol"`
	Action       ExitAction `json:"action"`
	SellPrice    float64    `json:"sell_price"`    // Limit price to place
	SellQuantity float64    `json:"sell_quantity"` // Qty to sell (partial or full)
	PnLPct       float64    `json:"pnl_pct"`
	Reason       string     `json:"reason"`
	Warning      bool       `json:"warning,omitempty"`
	WarningCode  string     `json:"warning_code,omitempty"`
	GeneratedAt  time.Time  `json:"generated_at"`
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
// Returns analysis decisions. Loss conditions are warning-only and never authorize a SELL.
// Automatic SELL authority is limited to exits at or above the position cost basis.
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
			Symbol:       sym,
			Action:       ExitHold,
			SellPrice:    currentPrice,
			SellQuantity: pos.Quantity,
			PnLPct:       finiteFloat(pnlPct),
			GeneratedAt:  now,
		}

		switch {
		// 0. Deep loss: analysis warning only. Hermes never opens a stop-loss SELL.
		case cfg.Exit.PanicSellPnLThreshold < 0 && pnlPct <= cfg.Exit.PanicSellPnLThreshold:
			decision.Action = ExitHold
			decision.SellQuantity = 0
			decision.Warning = true
			decision.WarningCode = "DEEP_LOSS_DCA_REVIEW"
			decision.Reason = fmt.Sprintf("cảnh báo lỗ sâu: lỗ %.2f%% đã chạm ngưỡng cảnh báo %.2f%%; chỉ phân tích để xem xét DCA, không tự động bán cắt lỗ",
				pnlPct*100, cfg.Exit.PanicSellPnLThreshold*100)

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
				if currentPrice < pos.AvgEntryPrice {
					decision.Action = ExitHold
					decision.SellQuantity = 0
					decision.Warning = true
					decision.WarningCode = "TRAIL_BELOW_COST_DCA_REVIEW"
					decision.Reason = fmt.Sprintf("cảnh báo giá giảm khỏi đỉnh nhưng còn dưới giá vốn: giá %.4f, giá vốn %.4f; không tự động bán cắt lỗ", currentPrice, pos.AvgEntryPrice)
				} else {
					decision.Action = ExitTrailingStop
					decision.SellQuantity = pos.Quantity
					decision.Reason = fmt.Sprintf("bảo vệ lợi nhuận: giá=%.4f <= ngưỡng theo đỉnh=%.4f (đỉnh=%.4f, khoảng lùi=%.1f%%)", currentPrice, trailStop, peak, cfg.Exit.TrailingDistancePct*100)
				}
			}

		// 3. Time stop: position too old and PnL not recovering
		case cfg.Exit.TimeStopDays > 0 && pos.OpenedAt > 0 && time.Unix(pos.OpenedAt, 0).Before(now.AddDate(0, 0, -cfg.Exit.TimeStopDays)):
			// Time-based exits may protect non-negative capital only. A losing position
			// produces evidence for DCA review but never an automatic stop-loss order.
			if pnlPct < 0 {
				decision.Action = ExitHold
				decision.SellQuantity = 0
				decision.Warning = true
				decision.WarningCode = "AGED_LOSS_DCA_REVIEW"
				decision.Reason = fmt.Sprintf("cảnh báo vị thế giữ quá %d ngày và đang lỗ %.2f%%; phân tích DCA, không tự động bán cắt lỗ", cfg.Exit.TimeStopDays, pnlPct*100)
			} else if pnlPct >= cfg.Exit.MinPnLForTimeStop {
				decision.Action = ExitTimeStop
				decision.SellQuantity = pos.Quantity
				decision.Reason = fmt.Sprintf("kết thúc vị thế không lỗ sau %d ngày: lãi %.2f%%", cfg.Exit.TimeStopDays, pnlPct*100)
			}
		}

		if decision.Warning {
			log.Printf("[ExitManager] %s warning-only %s: %s", sym, decision.WarningCode, decision.Reason)
		} else if decision.Action != ExitHold {
			log.Printf("[ExitManager] %s → %s: %s", sym, decision.Action, decision.Reason)
		}
		decisions = append(decisions, decision)
	}
	return decisions
}

// PlaceSellLimitOrder places a sell limit order for an exit decision.
func PlaceSellLimitOrder(ctx context.Context, decision ExitDecision, placer OrderPlacer) (live.OrderResult, error) {
	if decision.Warning || decision.PnLPct < 0 {
		return live.OrderResult{}, fmt.Errorf("automatic stop-loss sell forbidden: warning/DCA analysis only")
	}
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
	placeCtx, cancel := context.WithTimeout(ctx, managedExchangeTimeout)
	defer cancel()
	return placer.PlaceSpotLimitOrder(placeCtx, req)
}
