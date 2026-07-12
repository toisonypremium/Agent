package liveguard

import (
	"fmt"
	"math"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type ExecutionAssertionInput struct {
	Config               config.Config
	Plan                 agent2.Plan
	Desired              ManagedDesiredOrder
	OpenNotionalTotal    float64
	OpenNotionalBySymbol map[string]float64
	DryRun               bool
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
	if cfg.Live.FirstOrderRequireDryRun && !in.DryRun {
		reasons = append(reasons, "live.first_order_require_dry_run=true; dry-run audit required before first real order")
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
	out := []string{
		"plan=" + string(plan.State),
		"permission=" + string(plan.ActionPermission),
		"symbol=" + strings.ToUpper(desired.Symbol),
		"side=" + strings.ToUpper(desired.Side),
		"type=" + strings.ToLower(desired.Type),
		fmt.Sprintf("post_only=%v", desired.PostOnly),
		fmt.Sprintf("notional=%.2f", desired.Notional),
		fmt.Sprintf("layer=%d", desired.LayerIndex),
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
