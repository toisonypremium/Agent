package liveguard

import (
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type ManagedExecutionContext struct {
	BTCAccumulationPhase        string    `json:"btc_accumulation_phase,omitempty"`
	FirstOrderDryRunApproved    bool      `json:"first_order_dry_run_approved,omitempty"`
	ManagedOrderHistoryKnown    bool      `json:"managed_order_history_known,omitempty"`
	HasManagedRealOrderHistory  bool      `json:"has_managed_real_order_history,omitempty"`
	HermesMode                  string    `json:"hermes_mode,omitempty"`
	HermesDecisionID            string    `json:"hermes_decision_id,omitempty"`
	HermesIntent                string    `json:"hermes_intent,omitempty"`
	PortfolioLossStateKnown     bool      `json:"portfolio_loss_state_known,omitempty"`
	PortfolioLossLockActive     bool      `json:"portfolio_loss_lock_active,omitempty"`
	PortfolioLossDrawdownPct    float64   `json:"portfolio_loss_drawdown_pct,omitempty"`
	PortfolioLossStateUpdatedAt time.Time `json:"portfolio_loss_state_updated_at,omitempty"`
	DailyRealizedPnL            float64   `json:"daily_realized_pnl,omitempty"`
	DailyLossEquityBasis        float64   `json:"daily_loss_equity_basis,omitempty"`
	DailyRealizedLossLockActive bool      `json:"daily_realized_loss_lock_active,omitempty"`
}

type ExecutionAssertionInput struct {
	Config               config.Config
	Plan                 agent2.Plan
	Desired              ManagedDesiredOrder
	OpenNotionalTotal    float64
	OpenNotionalBySymbol map[string]float64
	DryRun               bool
	ManagedExecutionContext
}

func AssertManagedExecutionAllowed(in ExecutionAssertionInput) []string {
	cfg := in.Config
	d := in.Desired
	reasons := []string{}
	if !cfg.Live.Enabled {
		reasons = append(reasons, "live.enabled=false")
	}
	if !cfg.Live.AutoExecute {
		reasons = append(reasons, "live.auto_execute=false")
	}
	if cfg.Live.RequireManualConfirm {
		reasons = append(reasons, "live.require_manual_confirm=true")
	}
	if cfg.Live.ProofOnly {
		reasons = append(reasons, "live.proof_only=true")
	}
	if !cfg.Execution.RealTradingEnabled {
		reasons = append(reasons, "execution.real_trading_enabled=false")
	}
	if cfg.Live.FirstOrderRequireDryRun && !in.DryRun && !in.FirstOrderDryRunApproved {
		reasons = append(reasons, "live.first_order_require_dry_run=true; approved dry-run audit required before first real order")
	}
	if (cfg.Risk.MaxTotalEquityDrawdownPct > 0 || cfg.Risk.MaxDailyRealizedLossPct > 0) && !in.DryRun {
		if !in.PortfolioLossStateKnown {
			reasons = append(reasons, "portfolio loss state unavailable")
		} else {
			if cfg.Risk.MaxTotalEquityDrawdownPct > 0 && in.PortfolioLossLockActive {
				reasons = append(reasons, fmt.Sprintf("portfolio drawdown protection active (%.2f%%)", in.PortfolioLossDrawdownPct*100))
			}
			if cfg.Risk.MaxDailyRealizedLossPct > 0 && in.DailyRealizedLossLockActive {
				reasons = append(reasons, fmt.Sprintf("daily realized-loss protection active (PnL %.2f USDT)", in.DailyRealizedPnL))
			}
		}
	}
	if !cfg.Risk.NoFutures || !cfg.Risk.NoLeverage || !cfg.Risk.SpotLimitOnly {
		reasons = append(reasons, "risk flags must enforce no futures/no leverage/spot limit only")
	}
	if in.Plan.State != agent2.StateActiveLimit {
		reasons = append(reasons, "plan state must be ACTIVE_LIMIT")
	}
	if in.Plan.ActionPermission != agent1.Allowed {
		reasons = append(reasons, "plan action permission must be ALLOWED")
	}
	phase := strings.ToUpper(strings.TrimSpace(in.BTCAccumulationPhase))
	if phase == "" {
		if !in.DryRun {
			reasons = append(reasons, "BTC accumulation phase must be ACCUMULATION_CONFIRMED")
		}
	} else if phase != "ACCUMULATION_CONFIRMED" {
		reasons = append(reasons, "BTC accumulation phase must be ACCUMULATION_CONFIRMED")
	}
	if strings.EqualFold(d.Source, "HERMES_OPERATOR") {
		if strings.TrimSpace(in.HermesDecisionID) == "" {
			reasons = append(reasons, "Hermes decision_id required")
		}
		if strings.EqualFold(in.HermesIntent, "PROBE_LIMIT") && d.AllocationTier != string(OpportunityProbe) {
			reasons = append(reasons, "Hermes canary order must use PROBE allocation tier")
		}
	}
	if strings.ToUpper(d.Side) != "BUY" {
		reasons = append(reasons, "desired side must be BUY")
	}
	if strings.ToLower(d.Type) != "limit" {
		reasons = append(reasons, "desired type must be limit")
	}
	if !d.PostOnly {
		reasons = append(reasons, "desired order must be post-only")
	}
	if strings.TrimSpace(d.InstID) == "" {
		reasons = append(reasons, "desired inst_id required")
	}
	if !positiveFinite(d.Price) {
		reasons = append(reasons, "desired price must be positive")
	}
	if !positiveFinite(d.Quantity) {
		reasons = append(reasons, "desired quantity must be positive")
	}
	if !positiveFinite(d.Notional) {
		reasons = append(reasons, "desired notional must be positive")
	}
	if cap := normalizedMaxLiveNotionalPerOrder(cfg); cap > 0 && d.Notional > cap+1e-9 {
		reasons = append(reasons, fmt.Sprintf("desired notional %.2f above per-order cap %.2f", d.Notional, cap))
	}
	bySymbol := in.OpenNotionalBySymbol
	if bySymbol == nil {
		bySymbol = map[string]float64{}
	}
	symbol := strings.ToUpper(d.Symbol)
	if cap := normalizedMaxLiveNotionalPerAsset(cfg); cap > 0 && bySymbol[symbol]+d.Notional > cap+1e-9 {
		reasons = append(reasons, fmt.Sprintf("desired notional would exceed per-asset cap %.2f", cap))
	}
	if cap := normalizedMaxLiveNotionalTotal(cfg); cap > 0 && in.OpenNotionalTotal+d.Notional > cap+1e-9 {
		reasons = append(reasons, fmt.Sprintf("desired notional would exceed total cap %.2f", cap))
	}
	return uniqueStrings(reasons)
}

func FinalAssertionAudit(plan agent2.Plan, desired ManagedDesiredOrder, blockers []string) []string {
	return FinalAssertionAuditWithContext(ManagedExecutionContext{}, plan, desired, blockers)
}

func FinalAssertionAuditWithContext(execCtx ManagedExecutionContext, plan agent2.Plan, desired ManagedDesiredOrder, blockers []string) []string {
	out := []string{
		"plan=" + string(plan.State),
		"permission=" + string(plan.ActionPermission),
		"symbol=" + strings.ToUpper(desired.Symbol),
		"side=" + strings.ToUpper(desired.Side),
		"type=" + strings.ToLower(desired.Type),
		fmt.Sprintf("post_only=%v", desired.PostOnly),
		fmt.Sprintf("notional=%.2f", desired.Notional),
		fmt.Sprintf("layer=%d", desired.LayerIndex),
		"btc_accumulation=" + emptyAuditValue(execCtx.BTCAccumulationPhase),
		fmt.Sprintf("first_order_dry_run_approved=%v", execCtx.FirstOrderDryRunApproved),
	}
	if desired.AllocationTier != "" {
		out = append(out, "allocation_tier="+desired.AllocationTier)
	}
	if desired.QualityGrade != "" {
		out = append(out, "quality_grade="+desired.QualityGrade)
	}
	if len(blockers) > 0 {
		out = append(out, "assertion=BLOCK")
		out = append(out, blockers...)
	} else {
		out = append(out, "assertion=PASS")
	}
	return uniqueStrings(out)
}

func emptyAuditValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func positiveFinite(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func openNotionalMaps(openOrders []live.OrderStatus) (float64, map[string]float64) {
	total := 0.0
	bySymbol := map[string]float64{}
	for _, order := range openOrders {
		notional := order.Notional
		if notional <= 0 && order.Price > 0 && order.Quantity > 0 {
			notional = order.Price * order.Quantity
		}
		total += notional
		bySymbol[strings.ToUpper(orderSymbol(order))] += notional
	}
	return total, bySymbol
}
