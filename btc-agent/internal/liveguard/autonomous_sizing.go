package liveguard

import (
	"math"
	"strings"

	"btc-agent/internal/hermesoperator"
)

// AutonomousSizingContext contains market facts used to size strategy risk.
// System safety (halt, data, reconcile, OKX) remains in HermesSafetyContext.
type AutonomousSizingContext struct {
	TotalCapital       float64
	Confidence         float64
	Intent             hermesoperator.Intent
	AccumulationPhase  string
	MarketRegime       string
	TrendScore         float64
	MMConfidence       float64
	DataQuality        float64
	LiquidityQuality   float64
	RequestedNotional  float64
	PerOrderCap        float64
	AssetRemaining     float64
	PortfolioRemaining float64
}

type AutonomousSizingResult struct {
	CapitalPct       float64  `json:"capital_pct"`
	ConfidenceFactor float64  `json:"confidence_factor"`
	PhaseFactor      float64  `json:"phase_factor"`
	RegimeFactor     float64  `json:"regime_factor"`
	DataFactor       float64  `json:"data_factor"`
	LiquidityFactor  float64  `json:"liquidity_factor"`
	NotionalUSDT     float64  `json:"notional_usdt"`
	Reasons          []string `json:"reasons,omitempty"`
}

// CalculateAutonomousSizing converts AI confidence and deterministic market facts
// into capital-at-risk. Market weakness reduces size instead of blocking. Only
// system-level safety may block execution.
func CalculateAutonomousSizing(in AutonomousSizingContext) AutonomousSizingResult {
	out := AutonomousSizingResult{}
	if in.TotalCapital <= 0 {
		return out
	}
	confidence := clampSizing(in.Confidence, 0, 1)
	if confidence == 0 {
		return out
	}
	out.ConfidenceFactor = math.Pow(confidence, 1.5)

	phase := strings.ToUpper(strings.TrimSpace(in.AccumulationPhase))
	switch phase {
	case "ACCUMULATION_CONFIRMED":
		out.PhaseFactor = 1.0
	case "RECLAIM":
		out.PhaseFactor = 0.65
	case "SELL_ABSORPTION":
		out.PhaseFactor = 0.35
	case "LIQUIDITY_SWEEP":
		out.PhaseFactor = 0.22
	case "MARKDOWN":
		out.PhaseFactor = 0.12
	default:
		out.PhaseFactor = 0.08
	}
	// MM confidence can lift early accumulation sizing, but never above confirmed.
	mm := clampSizing(in.MMConfidence, 0, 1)
	if mm > out.PhaseFactor {
		out.PhaseFactor = math.Min(0.75, (out.PhaseFactor+mm)/2)
	}

	switch strings.ToUpper(strings.TrimSpace(in.MarketRegime)) {
	case "ACCUMULATION", "WEAK_UPTREND":
		out.RegimeFactor = 1.0
	case "RANGE":
		out.RegimeFactor = 0.75
	case "DOWNTREND":
		out.RegimeFactor = 0.40
	case "PANIC_SELLING":
		out.RegimeFactor = 0.05
	default:
		out.RegimeFactor = 0.55
	}

	out.DataFactor = clampSizing(in.DataQuality, 0, 1)
	if out.DataFactor == 0 {
		out.DataFactor = 0.5
	}
	out.LiquidityFactor = clampSizing(in.LiquidityQuality, 0, 1)
	if out.LiquidityFactor == 0 {
		out.LiquidityFactor = 0.5
	}

	// Intent controls max fraction of capital per action.
	basePct := 0.0
	switch in.Intent {
	case hermesoperator.IntentProbeLimit:
		basePct = 0.05
	case hermesoperator.IntentOpenLimit:
		basePct = 0.18
	case hermesoperator.IntentScaleLimit:
		basePct = 0.12
	default:
		return out
	}

	out.CapitalPct = basePct * out.ConfidenceFactor * out.PhaseFactor * out.RegimeFactor * out.DataFactor * out.LiquidityFactor
	// Avoid meaningless dust while respecting the exchange/preflight minimum later.
	if out.CapitalPct < 0.002 && in.Intent == hermesoperator.IntentProbeLimit && confidence >= 0.60 && out.RegimeFactor > 0 {
		out.CapitalPct = 0.002
	}
	out.NotionalUSDT = in.TotalCapital * out.CapitalPct
	// AI requested amount is a ceiling, not the sizing source.
	if in.RequestedNotional > 0 && out.NotionalUSDT > in.RequestedNotional {
		out.NotionalUSDT = in.RequestedNotional
	}
	for _, cap := range []float64{in.PerOrderCap, in.AssetRemaining, in.PortfolioRemaining} {
		if cap > 0 && out.NotionalUSDT > cap {
			out.NotionalUSDT = cap
		}
	}
	if out.NotionalUSDT < 0 || math.IsNaN(out.NotionalUSDT) || math.IsInf(out.NotionalUSDT, 0) {
		out.NotionalUSDT = 0
	}
	return out
}

func clampSizing(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
