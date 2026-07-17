package storage

import (
	"strings"
	"time"
)

type HermesAssetPerformance struct {
	Symbol       string    `json:"symbol"`
	ClosedFills  int       `json:"closed_fills"`
	WinningFills int       `json:"winning_fills"`
	LosingFills  int       `json:"losing_fills"`
	RealizedPnL  float64   `json:"realized_pnl"`
	ClosedCost   float64   `json:"closed_cost"`
	WinRate      float64   `json:"win_rate"`
	Expectancy   float64   `json:"expectancy"`
	LastCloseAt  time.Time `json:"last_close_at,omitempty"`
}

type HermesLossProtection struct {
	ConsecutiveLosses  int                               `json:"consecutive_losses"`
	LastLossAt         time.Time                         `json:"last_loss_at,omitempty"`
	RollingRealizedPnL float64                           `json:"rolling_realized_pnl"`
	ClosedSellFills    int                               `json:"closed_sell_fills"`
	PeakRealizedPnL    float64                           `json:"peak_realized_pnl"`
	RealizedDrawdown   float64                           `json:"realized_drawdown"`
	MaxDrawdown        float64                           `json:"max_drawdown"`
	LastCloseAt        time.Time                         `json:"last_close_at,omitempty"`
	BySymbol           map[string]HermesAssetPerformance `json:"by_symbol,omitempty"`
}

// HermesLossProtectionSnapshot replays owned fills in chronological order and
// derives realized PnL without trusting mutable position rows. Fee values are
// included when denominated in USDT; other fee currencies remain in the ledger.
func (d *DB) HermesLossProtectionSnapshot(since time.Time) (HermesLossProtection, error) {
	rows, err := d.Query(`SELECT e.timestamp,e.symbol,UPPER(e.side),e.delta_quantity,e.fill_price,e.notional_delta,e.fee_delta,e.fee_currency FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' ORDER BY e.timestamp,e.id`)
	if err != nil {
		return HermesLossProtection{}, err
	}
	defer rows.Close()
	type inventory struct{ qty, cost float64 }
	inv := map[string]inventory{}
	out := HermesLossProtection{BySymbol: map[string]HermesAssetPerformance{}}
	for rows.Next() {
		var ts int64
		var symbol, side, feeCCY string
		var qty, price, notional, fee float64
		if err := rows.Scan(&ts, &symbol, &side, &qty, &price, &notional, &fee, &feeCCY); err != nil {
			return out, err
		}
		symbol = strings.ToUpper(symbol)
		v := inv[symbol]
		if notional <= 0 {
			notional = qty * price
		}
		baseCCY := strings.TrimSuffix(symbol, "USDT")
		feeUSDT := fee
		if strings.EqualFold(feeCCY, baseCCY) {
			feeUSDT = fee * price
		}
		if side == "BUY" {
			v.qty += qty
			v.cost += notional
			// OKX reports paid USDT fees as negative. Add their absolute cost
			// to inventory so close PnL includes both entry and exit fees.
			if strings.EqualFold(feeCCY, "USDT") || strings.EqualFold(feeCCY, baseCCY) {
				v.cost -= feeUSDT
			}
			inv[symbol] = v
			continue
		}
		if side != "SELL" || qty <= 0 || v.qty <= 0 {
			continue
		}
		sellQty := qty
		if sellQty > v.qty {
			sellQty = v.qty
		}
		avg := v.cost / v.qty
		proceeds := sellQty * price
		if price <= 0 && qty > 0 {
			proceeds = notional * sellQty / qty
		}
		pnl := proceeds - avg*sellQty
		if strings.EqualFold(feeCCY, "USDT") || strings.EqualFold(feeCCY, baseCCY) {
			pnl += feeUSDT
		}
		v.qty -= sellQty
		v.cost -= avg * sellQty
		if v.qty < 1e-12 {
			v = inventory{}
		}
		inv[symbol] = v
		// Build inventory from all history, but score only closes inside the
		// requested protection window. This includes positions opened earlier.
		if ts < since.Unix() {
			continue
		}
		out.RollingRealizedPnL += pnl
		out.ClosedSellFills++
		perf := out.BySymbol[symbol]
		perf.Symbol = symbol
		perf.ClosedFills++
		perf.RealizedPnL += pnl
		perf.ClosedCost += avg * sellQty
		perf.LastCloseAt = time.Unix(ts, 0)
		if pnl > 0 {
			perf.WinningFills++
		} else if pnl < 0 {
			perf.LosingFills++
		}
		perf.WinRate = float64(perf.WinningFills) / float64(perf.ClosedFills)
		if perf.ClosedCost > 0 {
			perf.Expectancy = perf.RealizedPnL / perf.ClosedCost
		}
		out.BySymbol[symbol] = perf
		out.LastCloseAt = time.Unix(ts, 0)
		if out.RollingRealizedPnL > out.PeakRealizedPnL {
			out.PeakRealizedPnL = out.RollingRealizedPnL
		}
		out.RealizedDrawdown = out.PeakRealizedPnL - out.RollingRealizedPnL
		if out.RealizedDrawdown > out.MaxDrawdown {
			out.MaxDrawdown = out.RealizedDrawdown
		}
		if pnl < 0 {
			out.ConsecutiveLosses++
			out.LastLossAt = time.Unix(ts, 0)
		} else if pnl > 0 {
			out.ConsecutiveLosses = 0
		}
	}
	return out, rows.Err()
}
