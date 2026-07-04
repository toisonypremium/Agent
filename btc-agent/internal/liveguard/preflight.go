package liveguard

import (
	"context"
	"math"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type FilterReader interface {
	InstrumentFilters(ctx context.Context) ([]live.InstrumentFilter, error)
}

type PreflightResult struct {
	Enabled     bool     `json:"enabled"`
	Pass        bool     `json:"pass"`
	Symbol      string   `json:"symbol"`
	InstID      string   `json:"inst_id"`
	Price       float64  `json:"price"`
	Quantity    float64  `json:"quantity"`
	Notional    float64  `json:"notional"`
	TickSize    float64  `json:"tick_size"`
	StepSize    float64  `json:"step_size"`
	MinSize     float64  `json:"min_size"`
	MinNotional float64  `json:"min_notional"`
	Reasons     []string `json:"reasons,omitempty"`
	Canary      bool     `json:"canary,omitempty"`
}

func RunPreflight(cfg config.Config, candidate CandidateOrder, filters []live.InstrumentFilter) (CandidateOrder, PreflightResult) {
	result := PreflightResult{Enabled: true, Symbol: candidate.Symbol, Price: candidate.Price, Quantity: candidate.Quantity, Notional: candidate.Notional, Canary: candidate.Canary}
	reasons := []string{}
	if candidate.Symbol == "" {
		reasons = append(reasons, "symbol is empty")
	}
	if candidate.Side != "BUY" {
		reasons = append(reasons, "side must be BUY")
	}
	if strings.ToLower(candidate.Type) != "limit" {
		reasons = append(reasons, "type must be limit")
	}
	if cfg.Live.RequirePostOnly && !candidate.PostOnly {
		reasons = append(reasons, "post_only required")
	}
	filter, ok := findFilter(candidate.Symbol, filters)
	if !ok {
		reasons = append(reasons, "instrument filter not found")
		result.Reasons = uniqueStrings(reasons)
		return candidate, result
	}
	result.InstID = filter.InstID
	result.TickSize = filter.TickSize
	result.StepSize = filter.StepSize
	result.MinSize = filter.MinSize
	result.MinNotional = filter.MinNotional
	if candidate.Price <= 0 || candidate.Quantity <= 0 || candidate.Notional <= 0 {
		reasons = append(reasons, "price quantity notional must be positive")
	}
	if filter.TickSize > 0 {
		candidate.Price = floorToStep(candidate.Price, filter.TickSize)
	}
	if cfg.Live.CanaryMode && cfg.Live.CanaryMaxNotionalUSDT > 0 {
		if candidate.Price*candidate.Quantity > cfg.Live.CanaryMaxNotionalUSDT {
			if candidate.Price > 0 {
				candidate.Quantity = cfg.Live.CanaryMaxNotionalUSDT / candidate.Price
			}
		}
	}
	if filter.StepSize > 0 {
		candidate.Quantity = floorToStep(candidate.Quantity, filter.StepSize)
	}
	candidate.Notional = candidate.Price * candidate.Quantity
	result.Price = candidate.Price
	result.Quantity = candidate.Quantity
	result.Notional = candidate.Notional
	if candidate.Price <= 0 || candidate.Quantity <= 0 || candidate.Notional <= 0 {
		reasons = append(reasons, "rounded price quantity notional must be positive")
	}
	if filter.MinSize > 0 && candidate.Quantity < filter.MinSize {
		reasons = append(reasons, "quantity below min_size")
	}
	if filter.MinNotional > 0 && candidate.Notional < filter.MinNotional {
		reasons = append(reasons, "notional below min_notional")
	}
	if cfg.Live.MaxOrderNotionalUSDT > 0 && candidate.Notional > cfg.Live.MaxOrderNotionalUSDT+1e-9 {
		reasons = append(reasons, "notional above live max")
	}
	if cfg.Live.CanaryMode && cfg.Live.CanaryMaxNotionalUSDT > 0 && candidate.Notional > cfg.Live.CanaryMaxNotionalUSDT+1e-9 {
		reasons = append(reasons, "notional above canary max")
	}
	result.Reasons = uniqueStrings(reasons)
	result.Pass = len(result.Reasons) == 0
	return candidate, result
}

func findFilter(symbol string, filters []live.InstrumentFilter) (live.InstrumentFilter, bool) {
	want := strings.ToUpper(symbol)
	wantInst := live.OKXInstID(symbol)
	for _, f := range filters {
		if strings.EqualFold(f.Symbol, want) || strings.EqualFold(f.InstID, wantInst) {
			return f, true
		}
	}
	return live.InstrumentFilter{}, false
}

func floorToStep(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	return math.Floor((v/step)+1e-12) * step
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
