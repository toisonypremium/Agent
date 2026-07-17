package hermesoperator

import (
	"math"
	"sort"
	"strings"
)

const ReasoningVersion = "hermes-quant-v1"

type QuantReasoningInput struct {
	Symbol                                                        string
	ProbeEligible                                                 bool
	SetupScore, MMScore, FlowScore, RotationScore, LiquidityScore float64
	RewardRisk, EntryPrice, Invalidation, Target                  float64
	MarketRegime, AccumulationPhase                               string
	DataQuality                                                   float64
	HistoricalTrades, HistoricalWins                              int
	HistoricalExpectancy                                          float64
	TotalCapital, MaxProbeCapitalPct, MaxProbeNotional            float64
}

type QuantReasoningResult struct {
	Version                    string             `json:"version"`
	Recommendation             Intent             `json:"recommendation"`
	Eligible                   bool               `json:"eligible"`
	EvidenceScore              float64            `json:"evidence_score"`
	PosteriorWinProbability    float64            `json:"posterior_win_probability"`
	ConservativeWinProbability float64            `json:"conservative_win_probability"`
	UncertaintyPenalty         float64            `json:"uncertainty_penalty"`
	ExpectedR                  float64            `json:"expected_r"`
	FractionalKelly            float64            `json:"fractional_kelly"`
	SuggestedCapitalPct        float64            `json:"suggested_capital_pct"`
	SuggestedNotionalUSDT      float64            `json:"suggested_notional_usdt"`
	Confidence                 float64            `json:"confidence"`
	EntryPrice                 float64            `json:"entry_price,omitempty"`
	Invalidation               float64            `json:"invalidation,omitempty"`
	Target                     float64            `json:"target,omitempty"`
	Features                   map[string]float64 `json:"features"`
	Reasons                    []string           `json:"reasons"`
}

// ComputeQuantReasoning is deterministic and has no order authority. It fuses
// independent evidence, shrinks sparse realized history toward a neutral prior,
// evaluates downside-adjusted expected R, and sizes with quarter Kelly under
// strategy-regime caps. System safety and lifecycle gates remain authoritative.
func ComputeQuantReasoning(in QuantReasoningInput) QuantReasoningResult {
	setup, mm, flow := q01(in.SetupScore), q01(in.MMScore/100), q01(in.FlowScore)
	rotation, liquidity := q01(in.RotationScore), q01(in.LiquidityScore/100)
	rrQuality := 0.0
	if in.RewardRisk > 0 {
		rrQuality = q01(1 - math.Exp(-in.RewardRisk/3))
	}
	evidence := .28*setup + .16*mm + .14*flow + .10*rotation + .12*liquidity + .20*rrQuality
	evidenceP := .25 + .55*evidence
	histP := .5
	if in.HistoricalTrades > 0 {
		histP = float64(in.HistoricalWins+2) / float64(in.HistoricalTrades+4)
	}
	histWeight := math.Min(.45, float64(in.HistoricalTrades)/20)
	posterior := evidenceP*(1-histWeight) + histP*histWeight
	if in.HistoricalTrades >= 3 {
		posterior += clamp(in.HistoricalExpectancy*1.5, -.08, .08)
	}
	posterior = clamp(posterior, .05, .95)
	dataQuality := q01(in.DataQuality)
	if dataQuality == 0 {
		dataQuality = .5
	}
	uncertainty := .06 + .12/math.Sqrt(float64(in.HistoricalTrades+1)) + .10*(1-dataQuality)
	conservative := clamp(posterior-uncertainty, .03, .90)
	expectedR := conservative*in.RewardRisk - (1 - conservative)
	kelly := 0.0
	if in.RewardRisk > 0 {
		kelly = conservative - (1-conservative)/in.RewardRisk
	}
	kelly = math.Max(0, kelly) * .25
	weakness := regimeFactor(in.MarketRegime) * phaseFactor(in.AccumulationPhase)
	capPct := in.MaxProbeCapitalPct
	if capPct <= 0 {
		capPct = .05
	}
	capitalPct := math.Min(capPct, kelly*weakness)
	notional := math.Max(0, in.TotalCapital*capitalPct)
	if in.MaxProbeNotional > 0 {
		notional = math.Min(notional, in.MaxProbeNotional)
	}
	confidence := q01(.35 + .35*evidence + .20*math.Min(1, math.Max(0, expectedR)/3) + .10*dataQuality - uncertainty*.25)
	eligible := in.ProbeEligible && in.RewardRisk > 0 && in.EntryPrice > in.Invalidation && in.Target > in.EntryPrice && expectedR > .25 && conservative >= .20 && notional > 0
	recommendation := IntentWatch
	reasons := []string{"deterministic evidence fusion", "Bayesian sparse-history shrinkage", "conservative expected-R and quarter-Kelly sizing"}
	if eligible {
		recommendation = IntentProbeLimit
		reasons = append(reasons, "positive downside-adjusted utility inside probe envelope")
	} else {
		reasons = append(reasons, "probe envelope or conservative utility not sufficient")
	}
	features := map[string]float64{"setup": setup, "mm": mm, "flow": flow, "rotation": rotation, "liquidity": liquidity, "rr_quality": rrQuality, "history_weight": histWeight, "regime_factor": regimeFactor(in.MarketRegime), "phase_factor": phaseFactor(in.AccumulationPhase), "data_quality": dataQuality}
	return QuantReasoningResult{Version: ReasoningVersion, Recommendation: recommendation, Eligible: eligible, EvidenceScore: evidence, PosteriorWinProbability: posterior, ConservativeWinProbability: conservative, UncertaintyPenalty: uncertainty, ExpectedR: expectedR, FractionalKelly: kelly, SuggestedCapitalPct: capitalPct, SuggestedNotionalUSDT: notional, Confidence: confidence, EntryPrice: in.EntryPrice, Invalidation: in.Invalidation, Target: in.Target, Features: sortedFeatures(features), Reasons: reasons}
}

func regimeFactor(s string) float64 {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ACCUMULATION", "WEAK_UPTREND":
		return 1
	case "RANGE":
		return .75
	case "DOWNTREND", "MARKDOWN":
		return .35
	case "PANIC_SELLING":
		return 0
	default:
		return .55
	}
}
func phaseFactor(s string) float64 {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ACCUMULATION_CONFIRMED":
		return 1
	case "RECLAIM":
		return .7
	case "SELL_ABSORPTION":
		return .45
	case "LIQUIDITY_SWEEP":
		return .3
	case "MARKDOWN":
		return .18
	default:
		return .25
	}
}
func q01(v float64) float64 { return clamp(v, 0, 1) }
func clamp(v, a, b float64) float64 {
	if v < a {
		return a
	}
	if v > b {
		return b
	}
	return v
}
func sortedFeatures(in map[string]float64) map[string]float64 {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]float64{}
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}
