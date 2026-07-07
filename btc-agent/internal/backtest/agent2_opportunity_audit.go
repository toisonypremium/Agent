package backtest

import (
	"fmt"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
)

const (
	OpportunityVerdictTuneReview = "TUNE_REVIEW"
	OpportunityVerdictWaitMarket = "WAIT_MARKET"
	OpportunityVerdictHoldStrict = "HOLD_STRICT"
)

type Agent2OpportunityAuditConfig struct {
	MinWindow1D   int      `json:"min_window_1d"`
	TargetSymbols []string `json:"target_symbols"`
}

type Agent2OpportunityAuditResult struct {
	Enabled bool                        `json:"enabled"`
	Rows    []Agent2OpportunityAuditRow `json:"rows"`
	Summary string                      `json:"summary"`
}

type Agent2OpportunityAuditRow struct {
	Symbol              string         `json:"symbol"`
	Samples             int            `json:"samples"`
	ActiveLimitCount    int            `json:"active_limit_count"`
	NearMissCount       int            `json:"near_miss_count"`
	BTCWaitRate         float64        `json:"btc_wait_rate"`
	FlowMissingRate     float64        `json:"flow_missing_rate"`
	MMMissingRate       float64        `json:"mm_missing_rate"`
	DiscountFailRate    float64        `json:"discount_fail_rate"`
	RewardRiskFailRate  float64        `json:"reward_risk_fail_rate"`
	RotationFailRate    float64        `json:"rotation_fail_rate"`
	FallingKnifeRate    float64        `json:"falling_knife_rate"`
	AvgSetupScore       float64        `json:"avg_setup_score"`
	AvgDiscountGapPct   float64        `json:"avg_discount_gap_pct"`
	AvgRewardRiskGap    float64        `json:"avg_reward_risk_gap"`
	TopMissingGate      string         `json:"top_missing_gate"`
	MissingGateCounts   map[string]int `json:"missing_gate_counts"`
	RecommendedAction   string         `json:"recommended_action"`
	ResearchOnlyVerdict string         `json:"research_only_verdict"`
}

type opportunityAcc struct {
	samples            int
	active             int
	near               int
	btcWait            int
	flowMissing        int
	mmMissing          int
	discountFail       int
	rewardRiskFail     int
	rotationFail       int
	fallingKnife       int
	setupScoreTotal    float64
	discountGapTotal   float64
	rewardRiskGapTotal float64
	missing            map[string]int
}

func RunAgent2OpportunityAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg Agent2OpportunityAuditConfig) (Agent2OpportunityAuditResult, error) {
	auditCfg = normalizeAgent2OpportunityAuditConfig(cfg, auditCfg)
	btc1d := btc["1d"]
	need := auditCfg.MinWindow1D + 1
	if len(btc1d) < need {
		return Agent2OpportunityAuditResult{}, fmt.Errorf("not enough BTC 1d candles for Agent2 opportunity audit; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < auditCfg.MinWindow1D {
		return Agent2OpportunityAuditResult{}, fmt.Errorf("not enough aligned asset candles for Agent2 opportunity audit; need %d got %d", need, lastIndex+1)
	}
	acc := map[string]*opportunityAcc{}
	for _, sym := range auditCfg.TargetSymbols {
		acc[sym] = &opportunityAcc{missing: map[string]int{}}
	}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := auditCfg.MinWindow1D; i <= lastIndex; i++ {
		btcWindow := btcTimeframeWindow(btc, i)
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		assetWindows := map[string][]market.Candle{}
		for _, sym := range auditCfg.TargetSymbols {
			if len(assets[sym]) > i {
				assetWindows[sym] = assets[sym][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assetWindows, benchmarks)
		for _, asset := range plan.Assets {
			a := acc[asset.Symbol]
			if a == nil {
				continue
			}
			accumulateOpportunity(a, cfg, asset)
		}
	}
	result := Agent2OpportunityAuditResult{Enabled: true}
	for _, sym := range auditCfg.TargetSymbols {
		if a := acc[sym]; a != nil {
			result.Rows = append(result.Rows, finalizeOpportunityRow(sym, a))
		}
	}
	sortOpportunityRows(result.Rows)
	result.Summary = summarizeOpportunityAudit(result.Rows)
	return result, nil
}

func normalizeAgent2OpportunityAuditConfig(cfg config.Config, auditCfg Agent2OpportunityAuditConfig) Agent2OpportunityAuditConfig {
	if auditCfg.MinWindow1D <= 0 {
		auditCfg.MinWindow1D = 60
	}
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func accumulateOpportunity(a *opportunityAcc, cfg config.Config, asset agent2.AssetPlan) {
	a.samples++
	if asset.State == agent2.StateActiveLimit {
		a.active++
	}
	if asset.SetupScore >= 0.55 && asset.State != agent2.StateActiveLimit {
		a.near++
	}
	a.setupScoreTotal += asset.SetupScore
	a.discountGapTotal += asset.DiscountGapPct
	if asset.RewardRisk < cfg.Risk.MinRewardRisk {
		a.rewardRiskGapTotal += cfg.Risk.MinRewardRisk - asset.RewardRisk
	}
	seen := map[string]bool{}
	for _, gate := range asset.SetupGates {
		if gate.Pass {
			continue
		}
		seen[gate.Name] = true
		a.missing[gate.Name]++
	}
	for _, reason := range asset.Reasons {
		switch reason.Code {
		case agent2.ReasonBTCPermission, agent2.ReasonBTCDowntrend:
			seen[agent2.EntryCheckBTCPermission] = true
			a.missing[agent2.EntryCheckBTCPermission]++
		}
	}
	if seen[agent2.EntryCheckBTCPermission] {
		a.btcWait++
	}
	if seen[agent2.EntryCheckAssetFlowEntry] {
		a.flowMissing++
	}
	if seen[agent2.EntryCheckMMAccumulation] {
		a.mmMissing++
	}
	if seen[agent2.EntryCheckDiscountZone] {
		a.discountFail++
	}
	if seen[agent2.EntryCheckRewardRisk] {
		a.rewardRiskFail++
	}
	if seen[agent2.EntryCheckRotationScore] || seen[agent2.EntryCheckRotationRank] {
		a.rotationFail++
	}
	if seen[agent2.EntryCheckFallingKnife] {
		a.fallingKnife++
	}
}

func finalizeOpportunityRow(symbol string, a *opportunityAcc) Agent2OpportunityAuditRow {
	row := Agent2OpportunityAuditRow{Symbol: symbol, Samples: a.samples, ActiveLimitCount: a.active, NearMissCount: a.near, MissingGateCounts: copyIntMap(a.missing)}
	if a.samples > 0 {
		den := float64(a.samples)
		row.BTCWaitRate = float64(a.btcWait) / den
		row.FlowMissingRate = float64(a.flowMissing) / den
		row.MMMissingRate = float64(a.mmMissing) / den
		row.DiscountFailRate = float64(a.discountFail) / den
		row.RewardRiskFailRate = float64(a.rewardRiskFail) / den
		row.RotationFailRate = float64(a.rotationFail) / den
		row.FallingKnifeRate = float64(a.fallingKnife) / den
		row.AvgSetupScore = a.setupScoreTotal / den
		row.AvgDiscountGapPct = a.discountGapTotal / den
		row.AvgRewardRiskGap = a.rewardRiskGapTotal / den
	}
	row.TopMissingGate = topCountKey(a.missing)
	row.RecommendedAction, row.ResearchOnlyVerdict = opportunityRecommendation(row)
	return row
}

func opportunityRecommendation(row Agent2OpportunityAuditRow) (string, string) {
	if row.Samples == 0 {
		return "Collect more data before tuning; no production rule change.", OpportunityVerdictHoldStrict
	}
	if row.BTCWaitRate >= 0.60 {
		return "BTC permission/downtrend dominates; wait for better market or review BTC gate audit before changing asset rules.", OpportunityVerdictWaitMarket
	}
	if row.FlowMissingRate >= 0.60 || row.MMMissingRate >= 0.60 {
		return "Flow/MM entry is main bottleneck; review Asset Flow Entry audit and wait for reclaim/absorption evidence.", OpportunityVerdictWaitMarket
	}
	if row.DiscountFailRate >= 0.45 && row.AvgDiscountGapPct <= 0.15 {
		return "Many candidates are close to discount; review discount premium in backtest only before any config change.", OpportunityVerdictTuneReview
	}
	if row.RewardRiskFailRate >= 0.45 && row.AvgRewardRiskGap <= 0.75 {
		return "RR is near threshold; review layer depth/entry audit before changing min_reward_risk.", OpportunityVerdictTuneReview
	}
	if row.FallingKnifeRate >= 0.30 {
		return "Falling-knife risk is frequent; do not force entries, wait for stabilization/reclaim.", OpportunityVerdictHoldStrict
	}
	if row.RotationFailRate >= 0.45 {
		return "Rotation/rank is frequent bottleneck; review rotation evidence, keep rank fail soft unless backtest proves otherwise.", OpportunityVerdictTuneReview
	}
	return "No single safe tuning lever dominates; keep strict gates and collect more samples.", OpportunityVerdictHoldStrict
}

func sortOpportunityRows(rows []Agent2OpportunityAuditRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].NearMissCount != rows[j].NearMissCount {
			return rows[i].NearMissCount > rows[j].NearMissCount
		}
		if rows[i].ActiveLimitCount != rows[j].ActiveLimitCount {
			return rows[i].ActiveLimitCount > rows[j].ActiveLimitCount
		}
		return rows[i].Symbol < rows[j].Symbol
	})
}

func summarizeOpportunityAudit(rows []Agent2OpportunityAuditRow) string {
	if len(rows) == 0 {
		return "Agent2 opportunity audit produced no rows; not enough aligned candles."
	}
	totalSamples := 0
	totalActive := 0
	totalNear := 0
	for _, row := range rows {
		totalSamples += row.Samples
		totalActive += row.ActiveLimitCount
		totalNear += row.NearMissCount
	}
	best := rows[0]
	return fmt.Sprintf("Agent2 opportunity audit rows=%d samples=%d active_limit=%d near_miss=%d top_near=%s top_missing=%s verdict=%s", len(rows), totalSamples, totalActive, totalNear, best.Symbol, emptyDash(best.TopMissingGate), best.ResearchOnlyVerdict)
}
