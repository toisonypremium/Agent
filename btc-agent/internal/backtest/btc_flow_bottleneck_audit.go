package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	FlowComponentSweepLow                  = "SWEEP_LOW"
	FlowComponentReclaimSupport            = "RECLAIM_SUPPORT"
	FlowComponentFailedBreakdown           = "FAILED_BREAKDOWN"
	FlowComponentAbsorption                = "ABSORPTION"
	FlowComponentSweepHigh                 = "SWEEP_HIGH"
	FlowComponentRejectResistance          = "REJECT_RESISTANCE"
	FlowComponentFailedBreakout            = "FAILED_BREAKOUT"
	FlowComponentDistribution              = "DISTRIBUTION"
	FlowComponentNeutral                   = "FLOW_NEUTRAL"
	FlowComponentWeakScore                 = "FLOW_WEAK_SCORE"
	FlowComponentBullScoreNearAccumulation = "BULL_SCORE_NEAR_ACCUMULATION"
	FlowComponentBearScoreNearDistribution = "BEAR_SCORE_NEAR_DISTRIBUTION"
)

const (
	BTCFlowParamVerdictBaseline  = "BASELINE"
	BTCFlowParamVerdictTooSparse = "TOO_SPARSE"
	BTCFlowParamVerdictNoisy     = "NOISY"
	BTCFlowParamVerdictCandidate = "CANDIDATE"
)

type BTCFlowBottleneckAuditConfig struct {
	MinWindow1D int   `json:"min_window_1d"`
	HorizonDays []int `json:"horizon_days"`
}

type BTCFlowBottleneckAuditResult struct {
	Enabled       bool                       `json:"enabled"`
	ComponentRows []BTCFlowComponentAuditRow `json:"component_rows"`
	BiasRows      []BTCFlowBiasAuditRow      `json:"bias_rows"`
	ParamRows     []BTCFlowParamAuditRow     `json:"param_rows"`
	Summary       string                     `json:"summary"`
}

type BTCFlowComponentAuditRow struct {
	Component     string          `json:"component"`
	Count         int             `json:"count"`
	Rate          float64         `json:"rate"`
	AvgBullScore  float64         `json:"avg_bull_score"`
	AvgBearScore  float64         `json:"avg_bear_score"`
	AvgConfidence float64         `json:"avg_confidence"`
	AvgReturn     map[int]float64 `json:"avg_return"`
	WinRate       map[int]float64 `json:"win_rate"`
	WorstDrawdown map[int]float64 `json:"worst_drawdown"`
}

type BTCFlowBiasAuditRow struct {
	Bias          flow.Bias       `json:"bias"`
	Count         int             `json:"count"`
	Rate          float64         `json:"rate"`
	AvgBullScore  float64         `json:"avg_bull_score"`
	AvgBearScore  float64         `json:"avg_bear_score"`
	AvgConfidence float64         `json:"avg_confidence"`
	AvgReturn     map[int]float64 `json:"avg_return"`
	WinRate       map[int]float64 `json:"win_rate"`
	WorstDrawdown map[int]float64 `json:"worst_drawdown"`
}

type BTCFlowParamAuditRow struct {
	Name          string      `json:"name"`
	Params        flow.Params `json:"params"`
	Windows       int         `json:"windows"`
	SignalDensity float64     `json:"signal_density"`
	NeutralRate   float64     `json:"neutral_rate"`
	WeakScoreRate float64     `json:"weak_score_rate"`
	BullishRate   float64     `json:"bullish_rate"`
	BearishRate   float64     `json:"bearish_rate"`
	Verdict       string      `json:"verdict"`
}

type btcFlowAcc struct {
	count           int
	bullTotal       float64
	bearTotal       float64
	confidenceTotal float64
	returns         map[int]float64
	wins            map[int]int
	worstDD         map[int]float64
	initialized     map[int]bool
}

type btcFlowParamSet struct {
	name   string
	params flow.Params
}

func RunBTCFlowBottleneckAudit(btc map[string][]market.Candle, auditCfg BTCFlowBottleneckAuditConfig) (BTCFlowBottleneckAuditResult, error) {
	auditCfg = normalizeBTCFlowBottleneckAuditConfig(auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	need := auditCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return BTCFlowBottleneckAuditResult{}, fmt.Errorf("not enough BTC 1d candles for BTC flow bottleneck audit; need %d got %d", need, len(btc1d))
	}

	params := flow.DefaultParams()
	componentAcc := map[string]*btcFlowAcc{}
	biasAcc := map[flow.Bias]*btcFlowAcc{}
	for _, bias := range allBiases() {
		biasAcc[bias] = newBTCFlowAcc(auditCfg.HorizonDays)
	}
	windows := 0

	for i := auditCfg.MinWindow1D; i+maxH < len(btc1d); i++ {
		entry := btc1d[i].Close
		if entry <= 0 {
			continue
		}
		sig := flow.AnalyzeWithParams(btc1d[:i+1], "1d", 60, params)
		windows++
		bias := sig.FlowBias
		if bias == "" {
			bias = flow.BiasNeutral
		}
		if biasAcc[bias] == nil {
			biasAcc[bias] = newBTCFlowAcc(auditCfg.HorizonDays)
		}
		accumulateBTCFlowSignal(biasAcc[bias], sig, btc1d, i, auditCfg.HorizonDays)

		for _, component := range btcFlowComponents(sig, params) {
			if componentAcc[component] == nil {
				componentAcc[component] = newBTCFlowAcc(auditCfg.HorizonDays)
			}
			accumulateBTCFlowSignal(componentAcc[component], sig, btc1d, i, auditCfg.HorizonDays)
		}
	}

	result := BTCFlowBottleneckAuditResult{Enabled: true}
	for component, acc := range componentAcc {
		result.ComponentRows = append(result.ComponentRows, finalizeBTCFlowComponentRow(component, acc, auditCfg.HorizonDays, windows))
	}
	sortBTCFlowComponentRows(result.ComponentRows)
	for _, bias := range allBiases() {
		result.BiasRows = append(result.BiasRows, finalizeBTCFlowBiasRow(bias, biasAcc[bias], auditCfg.HorizonDays, windows))
	}
	result.ParamRows = runBTCFlowParamAudit(btc1d, auditCfg.MinWindow1D, params)
	result.Summary = summarizeBTCFlowBottleneckAudit(result.ComponentRows, result.BiasRows, result.ParamRows, windows)
	return result, nil
}

func normalizeBTCFlowBottleneckAuditConfig(auditCfg BTCFlowBottleneckAuditConfig) BTCFlowBottleneckAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	out := auditCfg.HorizonDays[:0]
	seen := map[int]bool{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	sort.Ints(out)
	auditCfg.HorizonDays = out
	return auditCfg
}

func newBTCFlowAcc(horizons []int) *btcFlowAcc {
	a := &btcFlowAcc{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func accumulateBTCFlowSignal(a *btcFlowAcc, sig flow.Signal, candles []market.Candle, index int, horizons []int) {
	a.count++
	a.bullTotal += sig.BullScore
	a.bearTotal += sig.BearScore
	a.confidenceTotal += sig.Confidence
	entry := candles[index].Close
	for _, h := range horizons {
		future := candles[index+h]
		ret := (future.Close - entry) / entry
		dd := worstDrawdown(candles[index+1:index+h+1], entry)
		a.returns[h] += ret
		if ret > 0 {
			a.wins[h]++
		}
		if !a.initialized[h] || dd < a.worstDD[h] {
			a.worstDD[h] = dd
			a.initialized[h] = true
		}
	}
}

func finalizeBTCFlowComponentRow(component string, a *btcFlowAcc, horizons []int, total int) BTCFlowComponentAuditRow {
	row := BTCFlowComponentAuditRow{Component: component, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	finalizeBTCFlowRow(a, horizons, total, &row.Count, &row.Rate, &row.AvgBullScore, &row.AvgBearScore, &row.AvgConfidence, row.AvgReturn, row.WinRate, row.WorstDrawdown)
	return row
}

func finalizeBTCFlowBiasRow(bias flow.Bias, a *btcFlowAcc, horizons []int, total int) BTCFlowBiasAuditRow {
	row := BTCFlowBiasAuditRow{Bias: bias, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
	finalizeBTCFlowRow(a, horizons, total, &row.Count, &row.Rate, &row.AvgBullScore, &row.AvgBearScore, &row.AvgConfidence, row.AvgReturn, row.WinRate, row.WorstDrawdown)
	return row
}

func finalizeBTCFlowRow(a *btcFlowAcc, horizons []int, total int, count *int, rate, bull, bear, confidence *float64, returns, winRate, worstDD map[int]float64) {
	if a == nil {
		return
	}
	*count = a.count
	if total > 0 {
		*rate = float64(a.count) / float64(total)
	}
	if a.count > 0 {
		*bull = a.bullTotal / float64(a.count)
		*bear = a.bearTotal / float64(a.count)
		*confidence = a.confidenceTotal / float64(a.count)
	}
	for _, h := range horizons {
		if a.count > 0 {
			returns[h] = a.returns[h] / float64(a.count)
			winRate[h] = float64(a.wins[h]) / float64(a.count)
			worstDD[h] = a.worstDD[h]
		}
	}
}

func btcFlowComponents(sig flow.Signal, params flow.Params) []string {
	out := []string{}
	if sig.SweepLow {
		out = append(out, FlowComponentSweepLow)
	}
	if sig.ReclaimSupport {
		out = append(out, FlowComponentReclaimSupport)
	}
	if sig.FailedBreakdown {
		out = append(out, FlowComponentFailedBreakdown)
	}
	if sig.Absorption {
		out = append(out, FlowComponentAbsorption)
	}
	if sig.SweepHigh {
		out = append(out, FlowComponentSweepHigh)
	}
	if sig.RejectResistance {
		out = append(out, FlowComponentRejectResistance)
	}
	if sig.FailedBreakout {
		out = append(out, FlowComponentFailedBreakout)
	}
	if sig.Distribution {
		out = append(out, FlowComponentDistribution)
	}
	if sig.FlowBias == flow.BiasNeutral || sig.FlowBias == "" {
		out = append(out, FlowComponentNeutral)
	}
	if sig.Confidence < 0.25 {
		out = append(out, FlowComponentWeakScore)
	}
	if sig.BullScore > 0 && sig.BullScore < params.AccumulationScore {
		out = append(out, FlowComponentBullScoreNearAccumulation)
	}
	if sig.BearScore > 0 && sig.BearScore < params.DistributionScore {
		out = append(out, FlowComponentBearScoreNearDistribution)
	}
	return uniqueStrings(out)
}

func uniqueStrings(in []string) []string {
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

func sortBTCFlowComponentRows(rows []BTCFlowComponentAuditRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Component < rows[j].Component
	})
}

func btcFlowParamSets() []btcFlowParamSet {
	base := flow.DefaultParams()
	looserVolume := base
	looserVolume.VolumeHighMultiplier = 1.10
	looserWick := base
	looserWick.WickRatio = 0.25
	looserAccum := base
	looserAccum.AccumulationScore = 0.25
	looserTrap := base
	looserTrap.TrapScore = 0.35
	balanced := base
	balanced.VolumeHighMultiplier = 1.10
	balanced.WickRatio = 0.25
	balanced.AccumulationScore = 0.25
	balanced.TrapScore = 0.35
	return []btcFlowParamSet{
		{name: "current", params: base},
		{name: "looser_volume", params: looserVolume},
		{name: "looser_wick", params: looserWick},
		{name: "looser_accum_score", params: looserAccum},
		{name: "looser_trap_score", params: looserTrap},
		{name: "balanced_looser", params: balanced},
	}
}

func runBTCFlowParamAudit(candles []market.Candle, minWindow int, base flow.Params) []BTCFlowParamAuditRow {
	rows := []BTCFlowParamAuditRow{}
	for _, set := range btcFlowParamSets() {
		row := BTCFlowParamAuditRow{Name: set.name, Params: set.params}
		for i := minWindow; i < len(candles); i++ {
			if candles[i].Close <= 0 {
				continue
			}
			sig := flow.AnalyzeWithParams(candles[:i+1], "1d", 60, set.params)
			row.Windows++
			if sig.FlowBias == flow.BiasNeutral || sig.FlowBias == "" {
				row.NeutralRate++
			} else {
				row.SignalDensity++
			}
			if sig.Confidence < 0.25 {
				row.WeakScoreRate++
			}
			if sig.FlowBias == flow.BiasAccumulation || sig.FlowBias == flow.BiasBearTrap {
				row.BullishRate++
			}
			if sig.FlowBias == flow.BiasDistribution || sig.FlowBias == flow.BiasBullTrap {
				row.BearishRate++
			}
		}
		if row.Windows > 0 {
			w := float64(row.Windows)
			row.SignalDensity /= w
			row.NeutralRate /= w
			row.WeakScoreRate /= w
			row.BullishRate /= w
			row.BearishRate /= w
		}
		row.Verdict = btcFlowParamVerdict(row, base)
		rows = append(rows, row)
	}
	return rows
}

func btcFlowParamVerdict(row BTCFlowParamAuditRow, base flow.Params) string {
	if row.Params == base {
		return BTCFlowParamVerdictBaseline
	}
	if row.SignalDensity < 0.03 {
		return BTCFlowParamVerdictTooSparse
	}
	if row.SignalDensity > 0.25 || (row.BullishRate > 0.20 && row.BearishRate > 0.20) {
		return BTCFlowParamVerdictNoisy
	}
	return BTCFlowParamVerdictCandidate
}

func summarizeBTCFlowBottleneckAudit(components []BTCFlowComponentAuditRow, biases []BTCFlowBiasAuditRow, params []BTCFlowParamAuditRow, windows int) string {
	if windows == 0 {
		return "BTC flow bottleneck audit produced no windows; not enough BTC candles."
	}
	neutralCount, neutralRate := 0, 0.0
	weakCount, weakRate := 0, 0.0
	for _, row := range components {
		switch row.Component {
		case FlowComponentNeutral:
			neutralCount = row.Count
			neutralRate = row.Rate
		case FlowComponentWeakScore:
			weakCount = row.Count
			weakRate = row.Rate
		}
	}
	topComponent := "none"
	if len(components) > 0 {
		topComponent = components[0].Component
	}
	bestParam := "current"
	bestVerdict := BTCFlowParamVerdictBaseline
	for _, row := range params {
		if row.Verdict == BTCFlowParamVerdictCandidate {
			bestParam = row.Name
			bestVerdict = row.Verdict
			break
		}
	}
	return fmt.Sprintf("BTC flow bottleneck audit windows=%d neutral=%d(%.1f%%) weak_score=%d(%.1f%%) top_component=%s best_param=%s verdict=%s", windows, neutralCount, neutralRate*100, weakCount, weakRate*100, topComponent, bestParam, bestVerdict)
}
