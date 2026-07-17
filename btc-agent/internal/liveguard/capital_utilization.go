package liveguard

import (
	"math"
	"strings"
)

type CapitalUtilizationInput struct {
	TotalCapital, ExistingExposure, OpenBuyNotional, ReserveCashRatio, HardExposureCap float64
	MarketRegime, AccumulationPhase                                                    string
	PanicSelling                                                                       bool
}
type CapitalUtilizationResult struct {
	TargetDeploymentPct     float64  `json:"target_deployment_pct"`
	TargetDeploymentUSDT    float64  `json:"target_deployment_usdt"`
	CurrentDeploymentUSDT   float64  `json:"current_deployment_usdt"`
	CurrentUtilizationPct   float64  `json:"current_utilization_pct"`
	ReserveCashUSDT         float64  `json:"reserve_cash_usdt"`
	DeploymentGapUSDT       float64  `json:"deployment_gap_usdt"`
	AvailableDeploymentUSDT float64  `json:"available_deployment_usdt"`
	State                   string   `json:"state"`
	Reasons                 []string `json:"reasons,omitempty"`
}

// EvaluateCapitalUtilization keeps capital productive when conditions improve,
// while treating cash as risk capacity during markdown/panic. It grants no
// order authority; opportunity, lifecycle and safety gates remain mandatory.
func EvaluateCapitalUtilization(in CapitalUtilizationInput) CapitalUtilizationResult {
	out := CapitalUtilizationResult{}
	if in.TotalCapital <= 0 {
		return out
	}
	reserveRatio := clampSizing(in.ReserveCashRatio, 0, .95)
	out.ReserveCashUSDT = in.TotalCapital * reserveRatio
	target := .45
	switch strings.ToUpper(strings.TrimSpace(in.MarketRegime)) {
	case "PANIC_SELLING":
		target = .15
	case "DOWNTREND":
		target = .45
	case "RANGE":
		target = .55
	case "WEAK_UPTREND":
		target = .65
	case "ACCUMULATION":
		target = .70
	}
	switch strings.ToUpper(strings.TrimSpace(in.AccumulationPhase)) {
	case "MARKDOWN":
		target = math.Min(target, .35)
	case "LIQUIDITY_SWEEP":
		target = math.Min(target, .40)
	case "SELL_ABSORPTION":
		target = math.Min(target, .50)
	case "RECLAIM":
		target = math.Min(target, .60)
	case "ACCUMULATION_CONFIRMED":
		target = math.Min(.70, math.Max(target, .65))
	}
	if in.PanicSelling {
		target = math.Min(target, .15)
	}
	maxInvestable := 1 - reserveRatio
	if target > maxInvestable {
		target = maxInvestable
	}
	out.TargetDeploymentPct = target
	out.CurrentDeploymentUSDT = math.Max(0, in.ExistingExposure) + math.Max(0, in.OpenBuyNotional)
	out.CurrentUtilizationPct = out.CurrentDeploymentUSDT / in.TotalCapital
	out.TargetDeploymentUSDT = in.TotalCapital * target
	if in.HardExposureCap > 0 && out.TargetDeploymentUSDT > in.HardExposureCap {
		out.TargetDeploymentUSDT = in.HardExposureCap
		out.TargetDeploymentPct = out.TargetDeploymentUSDT / in.TotalCapital
	}
	out.DeploymentGapUSDT = math.Max(0, out.TargetDeploymentUSDT-out.CurrentDeploymentUSDT)
	out.AvailableDeploymentUSDT = out.DeploymentGapUSDT
	switch {
	case out.DeploymentGapUSDT <= .005*in.TotalCapital:
		out.State = "TARGET_REACHED"
	case out.CurrentUtilizationPct < target*.5:
		out.State = "UNDERUTILIZED"
	default:
		out.State = "CAPACITY_AVAILABLE"
	}
	out.Reasons = []string{"cash deployment requires positive expected-R and lifecycle confirmation", "reserve cash and hard exposure cap preserved"}
	return out
}
