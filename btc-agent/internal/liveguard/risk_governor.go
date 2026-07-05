package liveguard

import (
	"fmt"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

const (
	RiskGovernorOK    = "RISK_OK"
	RiskGovernorWarn  = "RISK_WARN"
	RiskGovernorBlock = "RISK_BLOCK"
)

type RiskGovernorResult struct {
	GeneratedAt time.Time `json:"generated_at"`
	Status      string    `json:"status"`
	Blockers    []string  `json:"blockers,omitempty"`
	Warnings    []string  `json:"warnings,omitempty"`
	Summary     string    `json:"summary"`
}

func EvaluateRiskGovernor(cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan, open []live.OrderStatus, positions []live.LivePosition, dataHealth DataHealthResult, reconcile ReconcileSafetyResult) RiskGovernorResult {
	res := RiskGovernorResult{GeneratedAt: time.Now(), Status: RiskGovernorOK}
	if dataHealth.Status == DataHealthBlock {
		res.Blockers = append(res.Blockers, "data health block")
		res.Blockers = append(res.Blockers, dataHealth.Blockers...)
	}
	if reconcile.Status == ReconcileBlock {
		res.Blockers = append(res.Blockers, "reconciliation mismatch requires manual check")
		res.Blockers = append(res.Blockers, reconcile.Blockers...)
	}
	if analysis.MarketRegime == "PANIC_SELLING" {
		res.Blockers = append(res.Blockers, "BTC PANIC_SELLING risk governor block")
	}
	if analysis.FallingKnifeRisk == agent1.High {
		res.Blockers = append(res.Blockers, "BTC falling knife HIGH risk governor block")
	}
	if analysis.FomoRisk == agent1.High {
		res.Blockers = append(res.Blockers, "BTC FOMO HIGH risk governor block")
	}
	for _, position := range positions {
		if position.Quantity < 0 || position.CostBasis < 0 || (position.Quantity > 0 && position.AvgEntryPrice <= 0) {
			res.Blockers = append(res.Blockers, fmt.Sprintf("invalid live position exposure for %s", position.Symbol))
		}
	}
	if exposure := totalLiveExposure(open, positions); cfg.Live.MaxLiveNotionalTotalUSDT > 0 && exposure > cfg.Live.MaxLiveNotionalTotalUSDT+1e-9 {
		res.Blockers = append(res.Blockers, fmt.Sprintf("live exposure %.2f exceeds total cap %.2f", exposure, cfg.Live.MaxLiveNotionalTotalUSDT))
	}

	if analysis.MarketRegime == "DOWNTREND" {
		res.Warnings = append(res.Warnings, "BTC DOWNTREND: keep reduced risk budget")
	}
	if analysis.ActionPermission == agent1.Watch {
		res.Warnings = append(res.Warnings, "BTC permission WATCH: risk budget reduced")
	}
	// Armed = preparing for entry; not a risk reduction warning.
	if cfg.Live.MaxOpenLiveOrdersTotal > 0 && len(open) >= cfg.Live.MaxOpenLiveOrdersTotal-1 {
		res.Warnings = append(res.Warnings, "open live orders near total cap")
	}
	if plan.State == agent2.StateNoTrade && len(open) > 0 {
		res.Warnings = append(res.Warnings, "plan NO_TRADE while live orders remain open")
	}
	res.refreshSummary()
	return res
}

func (r *RiskGovernorResult) refreshSummary() {
	r.Blockers = uniqueHealthStrings(r.Blockers)
	r.Warnings = uniqueHealthStrings(r.Warnings)
	switch {
	case len(r.Blockers) > 0:
		r.Status = RiskGovernorBlock
	case len(r.Warnings) > 0:
		r.Status = RiskGovernorWarn
	default:
		r.Status = RiskGovernorOK
	}
	r.Summary = fmt.Sprintf("%s: blockers=%d warnings=%d", r.Status, len(r.Blockers), len(r.Warnings))
}

func totalLiveExposure(open []live.OrderStatus, positions []live.LivePosition) float64 {
	total := 0.0
	for _, order := range open {
		total += orderNotional(order)
	}
	for _, position := range positions {
		total += position.CostBasis
	}
	return total
}
