package liveguard

import (
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

const (
	DataHealthOK    = "DATA_HEALTH_OK"
	DataHealthWarn  = "DATA_HEALTH_WARN"
	DataHealthBlock = "DATA_HEALTH_BLOCK"

	HealthSeverityHard = "HARD"
	HealthSeveritySoft = "SOFT"
)

const (
	maxAnalysisAge = 6 * time.Hour
	maxPlanAge     = 6 * time.Hour
	max1DCandleAge = 48 * time.Hour
	min1DCandles   = 60
)

type DataHealthResult struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Status      string            `json:"status"`
	Checks      []DataHealthCheck `json:"checks"`
	Blockers    []string          `json:"blockers,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	Summary     string            `json:"summary"`
}

type DataHealthCheck struct {
	Name     string `json:"name"`
	Pass     bool   `json:"pass"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

func CheckDataHealth(cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan, assets map[string][]market.Candle, open []live.OrderStatus, positions []live.LivePosition, now time.Time) DataHealthResult {
	if now.IsZero() {
		now = time.Now()
	}
	res := DataHealthResult{GeneratedAt: now, Status: DataHealthOK}
	add := func(name, severity string, pass bool, reason string) {
		check := DataHealthCheck{Name: name, Pass: pass, Severity: severity, Reason: reason}
		res.Checks = append(res.Checks, check)
		if pass {
			return
		}
		if severity == HealthSeverityHard {
			res.Blockers = append(res.Blockers, reason)
		} else {
			res.Warnings = append(res.Warnings, reason)
		}
	}

	add("analysis_fresh", HealthSeverityHard, freshTime(analysis.Timestamp, now, maxAnalysisAge), staleReason("analysis", analysis.Timestamp, now, maxAnalysisAge))
	add("plan_fresh", HealthSeverityHard, freshTime(plan.Timestamp, now, maxPlanAge), staleReason("plan", plan.Timestamp, now, maxPlanAge))

	for _, symbol := range cfg.Data.Symbols.Assets {
		candles := assets[symbol]
		name := "candles_" + strings.ToUpper(symbol)
		if len(candles) < min1DCandles {
			add(name, HealthSeverityHard, false, fmt.Sprintf("%s has only %d 1D candles; need %d", symbol, len(candles), min1DCandles))
			continue
		}
		latest := candles[len(candles)-1]
		if latest.Close <= 0 || latest.High <= 0 || latest.Low <= 0 {
			add(name, HealthSeverityHard, false, fmt.Sprintf("%s latest 1D candle has invalid OHLC", symbol))
			continue
		}
		candleTime := latestUsableCandleTime(latest, now)
		add(name, HealthSeverityHard, freshTime(candleTime, now, max1DCandleAge), staleReason(symbol+" 1D candle", candleTime, now, max1DCandleAge))
	}

	// No open orders = normal (no positions yet); only validate orders that exist.
	for _, order := range open {
		name := "open_order_" + firstDataHealthID(order.ClientOrderID, order.OrderID, order.InstID)
		pass := order.InstID != "" && (order.ClientOrderID != "" || order.OrderID != "") && order.Price > 0 && order.Quantity > 0 && orderNotional(order) > 0
		add(name, HealthSeverityHard, pass, fmt.Sprintf("invalid live open order identifiers/price/quantity/notional: inst=%s clOrdId=%s ordId=%s", order.InstID, order.ClientOrderID, order.OrderID))
	}

	// No positions = normal (no fills yet); only validate positions that exist.
	for _, position := range positions {
		pass := position.Symbol != "" && position.Quantity >= 0 && position.CostBasis >= 0
		if position.Quantity > 0 {
			pass = pass && position.AvgEntryPrice > 0
		}
		add("position_"+strings.ToUpper(position.Symbol), HealthSeverityHard, pass, fmt.Sprintf("invalid live position: %s qty=%.8f cost=%.2f avg=%.8f", position.Symbol, position.Quantity, position.CostBasis, position.AvgEntryPrice))
	}

	// Watchlist candidates absence is normal during WATCH/NO_TRADE; skip soft warn.
	res.refreshSummary()
	return res
}

func (r *DataHealthResult) refreshSummary() {
	r.Blockers = uniqueHealthStrings(r.Blockers)
	r.Warnings = uniqueHealthStrings(r.Warnings)
	switch {
	case len(r.Blockers) > 0:
		r.Status = DataHealthBlock
	case len(r.Warnings) > 0:
		r.Status = DataHealthWarn
	default:
		r.Status = DataHealthOK
	}
	r.Summary = fmt.Sprintf("%s: checks=%d blockers=%d warnings=%d", r.Status, len(r.Checks), len(r.Blockers), len(r.Warnings))
}

func latestUsableCandleTime(c market.Candle, now time.Time) time.Time {
	if !c.CloseTime.IsZero() && !c.CloseTime.After(now) {
		return c.CloseTime
	}
	if !c.OpenTime.IsZero() && !c.OpenTime.After(now) {
		return c.OpenTime
	}
	return c.CloseTime
}

func freshTime(ts, now time.Time, maxAge time.Duration) bool {
	if ts.IsZero() {
		return false
	}
	age := now.Sub(ts)
	return age >= 0 && age <= maxAge
}

func staleReason(name string, ts, now time.Time, maxAge time.Duration) string {
	if ts.IsZero() {
		return name + " timestamp missing"
	}
	age := now.Sub(ts)
	if age < 0 {
		return fmt.Sprintf("%s timestamp is in the future", name)
	}
	return fmt.Sprintf("%s stale: age=%s max=%s", name, age.Round(time.Minute), maxAge)
}

func orderNotional(order live.OrderStatus) float64 {
	if order.Notional > 0 {
		return order.Notional
	}
	if order.Price > 0 && order.Quantity > 0 {
		return order.Price * order.Quantity
	}
	return 0
}

func firstDataHealthID(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func uniqueHealthStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
