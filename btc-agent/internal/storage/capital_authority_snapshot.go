package storage

import (
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/config"
)

type CapitalAuthoritySnapshot struct {
	GeneratedAt             time.Time `json:"generated_at"`
	AccountEquityUSDT       float64   `json:"account_equity_usdt"`
	PositionCostBasisUSDT   float64   `json:"position_cost_basis_usdt"`
	PositionMarketValueUSDT float64   `json:"position_market_value_usdt"`
	OpenBuyNotionalUSDT     float64   `json:"open_buy_notional_usdt"`
	ReserveRequiredUSDT     float64   `json:"reserve_required_usdt"`
	HardExposureCapUSDT     float64   `json:"hard_exposure_cap_usdt"`
	ExistingExposureUSDT    float64   `json:"existing_exposure_usdt"`
	ConditionalCapacityUSDT float64   `json:"conditional_capacity_usdt"`
	ExecutableNowUSDT       float64   `json:"executable_now_usdt"`
	Permission              string    `json:"permission"`
	Blockers                []string  `json:"blockers,omitempty"`
	Source                  string    `json:"source"`
}

func (d *DB) BuildCapitalAuthoritySnapshot(cfg config.Config, now time.Time) (CapitalAuthoritySnapshot, error) {
	out := CapitalAuthoritySnapshot{GeneratedAt: now.UTC(), Source: "SQLite equity/live_positions/live_orders/config"}
	equity, err := d.EquityRiskState()
	if err != nil || equity.CurrentEquity <= 0 {
		out.Permission, out.Blockers = "UNKNOWN", []string{"account equity unavailable"}
		if err != nil {
			return out, fmt.Errorf("capital authority equity: %w", err)
		}
		return out, fmt.Errorf("capital authority equity is not positive")
	}
	positions, err := d.LivePositions()
	if err != nil {
		return out, fmt.Errorf("capital authority positions: %w", err)
	}
	orders, err := d.OpenLiveOrdersDetailed()
	if err != nil {
		return out, fmt.Errorf("capital authority orders: %w", err)
	}
	out.AccountEquityUSDT = equity.CurrentEquity
	for _, position := range positions {
		if position.CostBasis > 0 && finiteCapital(position.CostBasis) {
			out.PositionCostBasisUSDT += position.CostBasis
		}
	}
	for _, order := range orders {
		if strings.EqualFold(order.Side, "BUY") && order.Notional > 0 && finiteCapital(order.Notional) {
			out.OpenBuyNotionalUSDT += order.Notional
		}
	}
	out.PositionMarketValueUSDT = out.PositionCostBasisUSDT // no trusted mark-price source in this snapshot
	out.ExistingExposureUSDT = out.PositionCostBasisUSDT
	out.ReserveRequiredUSDT = out.AccountEquityUSDT * clampCapital(cfg.Portfolio.ReserveCashRatio)
	out.HardExposureCapUSDT = config.EffectiveHermesPortfolioExposure(cfg)
	out.ConditionalCapacityUSDT = math.Max(0, math.Min(out.HardExposureCapUSDT, out.AccountEquityUSDT-out.ReserveRequiredUSDT)-out.ExistingExposureUSDT-out.OpenBuyNotionalUSDT)
	out.ExecutableNowUSDT = 0
	out.Permission = "BLOCKED"
	out.Blockers = []string{"market authority and plan gates are evaluated separately"}
	return out, nil
}

func finiteCapital(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func clampCapital(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > .95 {
		return .95
	}
	return value
}
