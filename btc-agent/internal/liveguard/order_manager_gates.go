package liveguard

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

func firstOrderQuarantineAppliesWithContext(openOrders []live.OrderStatus, positions []live.LivePosition, execCtx ManagedExecutionContext) bool {
	if execCtx.ManagedOrderHistoryKnown {
		return !execCtx.HasManagedRealOrderHistory
	}
	for _, order := range openOrders {
		if live.NormalizeOrderStatus(order.Status) != "" {
			return false
		}
	}
	for _, position := range positions {
		if position.Quantity > 0 || position.CostBasis > 0 {
			return false
		}
	}
	return true
}

func loadHistoryQualityScores(path string) map[string]historyQualityScore {
	out := map[string]historyQualityScore{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var report struct {
		PerCoin map[string]struct {
			QualityScore float64 `json:"quality_score"`
			QualityGrade string  `json:"quality_grade"`
		} `json:"per_coin"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return out
	}
	for symbol, stats := range report.PerCoin {
		out[strings.ToUpper(symbol)] = historyQualityScore{Score: stats.QualityScore, Grade: stats.QualityGrade}
	}
	return out
}

func shouldCancelOpenOrder(cfg config.Config, plan agent2.Plan, order live.OrderStatus, desired ManagedDesiredOrder) bool {
	if cfg.Live.CancelIfPlanNotActive && plan.State != agent2.StateActiveLimit {
		return true
	}
	if desired.Symbol == "" {
		return true
	}
	if priceDriftExceeded(cfg, order.Price, desired.Price) {
		return true
	}
	if cfg.Live.CancelIfPriceAboveDiscountZonePct > 0 && order.Price > desired.DiscountZone.High*(1+cfg.Live.CancelIfPriceAboveDiscountZonePct) {
		return true
	}
	if cfg.Live.CancelStaleAfterMinutes > 0 && order.SubmittedAt > 0 && time.Since(time.Unix(order.SubmittedAt, 0)) > time.Duration(cfg.Live.CancelStaleAfterMinutes)*time.Minute {
		return true
	}
	return false
}

func cancelReason(cfg config.Config, plan agent2.Plan, order live.OrderStatus, foundDesired bool) string {
	if cfg.Live.CancelIfPlanNotActive && plan.State != agent2.StateActiveLimit {
		return "plan no longer ACTIVE_LIMIT"
	}
	if !foundDesired {
		return "order no longer matches active asset/layer"
	}
	return "order no longer matches current desired layer"
}

func priceDriftExceeded(cfg config.Config, current, desired float64) bool {
	if cfg.Live.ReplaceIfPriceDriftPct <= 0 || current <= 0 || desired <= 0 {
		return false
	}
	return math.Abs(current-desired)/desired > cfg.Live.ReplaceIfPriceDriftPct
}

func haltedState(h HaltReader) (bool, error) {
	if h == nil {
		return true, fmt.Errorf("halt reader unavailable")
	}
	return h.IsHalted()
}

func managedKey(symbol string, layer int) string {
	return strings.ToUpper(symbol) + "#" + fmt.Sprint(layer)
}

func orderKey(o live.OrderStatus) string {
	if o.LayerIndex <= 0 {
		return ""
	}
	return managedKey(orderSymbol(o), o.LayerIndex)
}

func orderSymbol(o live.OrderStatus) string {
	if o.Symbol != "" {
		return strings.ToUpper(o.Symbol)
	}
	return live.InternalSymbol(o.InstID)
}

func matchBySymbolPrice(order live.OrderStatus, desired []ManagedDesiredOrder) (ManagedDesiredOrder, bool) {
	for _, d := range desired {
		if d.Symbol == orderSymbol(order) && math.Abs(order.Price-d.Price) <= math.Max(1e-9, d.Price*0.0001) {
			return d, true
		}
	}
	return ManagedDesiredOrder{}, false
}
func managedSummary(r ManagedCycleResult) string {
	if len(r.Reasons) > 0 {
		return r.Status + ": " + strings.Join(r.Reasons, "; ")
	}
	return fmt.Sprintf("%s: desired=%d kept=%d canceled=%d replaced=%d placed=%d blocked=%d", r.Status, len(r.Desired), len(r.Kept), len(r.Canceled), len(r.Replaced), len(r.Placed), len(r.Blocked))
}

func normalizedMaxAutoLayersPerAsset(cfg config.Config) int {
	if cfg.Live.MaxAutoLayersPerAsset > 0 {
		return minInt(cfg.Live.MaxAutoLayersPerAsset, 3)
	}
	return 1
}

func normalizedMaxOpenLiveOrdersPerAsset(cfg config.Config) int {
	if cfg.Live.MaxOpenLiveOrdersPerAsset > 0 {
		return cfg.Live.MaxOpenLiveOrdersPerAsset
	}
	return 1
}

func normalizedMaxOpenLiveOrdersTotal(cfg config.Config) int {
	if cfg.Live.MaxOpenLiveOrdersTotal > 0 {
		return cfg.Live.MaxOpenLiveOrdersTotal
	}
	return normalizedMaxOpenLiveOrdersPerAsset(cfg) * len(cfg.Data.Symbols.Assets)
}

func normalizedMaxLiveNotionalPerOrder(cfg config.Config) float64 {
	if v := config.EffectiveLiveNotionalPerOrder(cfg); v > 0 {
		return v
	}
	if cfg.Live.MaxLiveNotionalPerOrderUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerOrderUSDT
	}
	return cfg.Live.MaxOrderNotionalUSDT
}

func normalizedMaxLiveNotionalPerAsset(cfg config.Config) float64 {
	if v := config.EffectiveLiveNotionalPerAsset(cfg); v > 0 {
		return v
	}
	if cfg.Live.MaxLiveNotionalPerAssetUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerAssetUSDT
	}
	return normalizedMaxLiveNotionalPerOrder(cfg) * float64(normalizedMaxAutoLayersPerAsset(cfg))
}

func normalizedMaxLiveNotionalTotal(cfg config.Config) float64 {
	if v := config.EffectiveLiveNotionalTotal(cfg); v > 0 {
		return v
	}
	if cfg.Live.MaxLiveNotionalTotalUSDT > 0 {
		return cfg.Live.MaxLiveNotionalTotalUSDT
	}
	return normalizedMaxLiveNotionalPerAsset(cfg) * float64(len(cfg.Data.Symbols.Assets))
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
