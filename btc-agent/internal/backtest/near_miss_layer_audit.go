package backtest

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
)

const (
	NearMissLayerVerdictCandidate = "CANDIDATE"
	NearMissLayerVerdictWatch     = "WATCH"
	NearMissLayerVerdictReject    = "REJECT"
)

type NearMissLayerAuditConfig struct {
	ReadinessThresholds []float64 `json:"readiness_thresholds"`
	InvalidationBuffers []float64 `json:"invalidation_buffers"`
	TakeProfitPcts      []float64 `json:"take_profit_pcts"`
	TimeStopDays        []int     `json:"time_stop_days"`
	TargetSymbols       []string  `json:"target_symbols"`
}

type NearMissLayerAuditResult struct {
	Enabled bool                    `json:"enabled"`
	Rows    []NearMissLayerAuditRow `json:"rows"`
	Summary string                  `json:"summary"`
}

type NearMissLayerAuditRow struct {
	Symbol             string  `json:"symbol"`
	Trigger            string  `json:"trigger"`
	ReadinessThreshold float64 `json:"readiness_threshold"`
	InvalidationBuffer float64 `json:"invalidation_buffer"`
	TakeProfitPct      float64 `json:"take_profit_pct"`
	TimeStopDays       int     `json:"time_stop_days"`
	PlansCreated       int     `json:"plans_created"`
	OrdersPlaced       int     `json:"orders_placed"`
	OrdersFilled       int     `json:"orders_filled"`
	OrdersExpired      int     `json:"orders_expired"`
	TakeProfits        int     `json:"take_profits"`
	Invalidations      int     `json:"invalidations"`
	TimeStops          int     `json:"time_stops"`
	FillRate           float64 `json:"fill_rate"`
	MaxDeployed        float64 `json:"max_deployed"`
	MaxDrawdown        float64 `json:"max_drawdown"`
	FinalPnL           float64 `json:"final_pnl"`
	Score              float64 `json:"score"`
	Verdict            string  `json:"verdict"`
}

type nearMissLayerKey struct {
	symbol    string
	trigger   string
	threshold float64
	buffer    float64
	tp        float64
	timeStop  int
}

type nearMissLayerState struct {
	key       nearMissLayerKey
	sim       Agent2Simulation
	orders    map[string][]simOrder
	positions map[string]*simPosition
}

func RunNearMissLayerAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg NearMissLayerAuditConfig) (NearMissLayerAuditResult, error) {
	auditCfg = normalizeNearMissLayerAuditConfig(cfg, auditCfg)
	btc1d := btc["1d"]
	warmup := 60
	expiry := expiryDays(cfg.Execution.OrderExpiryHours)
	need := warmup + expiry + 1
	if len(btc1d) < need {
		return NearMissLayerAuditResult{}, fmt.Errorf("not enough BTC 1d candles for near-miss layer audit; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < need {
		return NearMissLayerAuditResult{}, fmt.Errorf("not enough aligned asset candles for near-miss layer audit; need %d got %d", need, lastIndex+1)
	}

	if len(auditCfg.TargetSymbols) == 0 {
		return NearMissLayerAuditResult{}, fmt.Errorf("no target symbols for near-miss layer audit")
	}
	states := map[nearMissLayerKey]*nearMissLayerState{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	targets := targetSet(auditCfg.TargetSymbols)
	for i := warmup; i <= lastIndex; i++ {
		btcWindow := map[string][]market.Candle{"1d": btc1d[:i+1], "4h": btc1d[:i+1], "1w": btc1d[:i+1]}
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
		for _, candidate := range plan.Watchlist.Candidates {
			if !targets[candidate.Symbol] || !nearMissLayerCandidateOK(candidate) {
				continue
			}
			trigger := watchlistTrigger(candidate)
			for _, threshold := range auditCfg.ReadinessThresholds {
				if candidate.ReadinessScore < threshold {
					continue
				}
				for _, buffer := range auditCfg.InvalidationBuffers {
					for _, tp := range auditCfg.TakeProfitPcts {
						for _, stop := range auditCfg.TimeStopDays {
							key := nearMissLayerKey{symbol: candidate.Symbol, trigger: trigger, threshold: threshold, buffer: buffer, tp: tp, timeStop: stop}
							state := states[key]
							if state == nil {
								state = newNearMissLayerState(key)
								states[key] = state
							}
							asset := forcedNearMissAssetPlan(cfg, candidate, buffer, i)
							placeOrders(&state.sim, state.orders, state.positions, agent2.Plan{Assets: []agent2.AssetPlan{asset}}, i, eventTime(btc1d[i].CloseTime))
						}
					}
				}
			}
		}
		for _, state := range states {
			processOrdersAndPositionsWithOverrides(&state.sim, state.orders, state.positions, assets, i, expiry, eventTime(btc1d[i].CloseTime), SimulationOverrides{TakeProfitPct: state.key.tp, TimeStopDays: state.key.timeStop})
		}
	}

	result := NearMissLayerAuditResult{Enabled: true}
	for _, state := range states {
		finalizeSimulation(&state.sim, state.positions, assets, lastIndex)
		stats := state.sim.Assets[state.key.symbol]
		row := NearMissLayerAuditRow{Symbol: state.key.symbol, Trigger: state.key.trigger, ReadinessThreshold: state.key.threshold, InvalidationBuffer: state.key.buffer, TakeProfitPct: state.key.tp, TimeStopDays: state.key.timeStop, PlansCreated: stats.PlansCreated, OrdersPlaced: stats.OrdersPlaced, OrdersFilled: stats.OrdersFilled, OrdersExpired: stats.OrdersExpired, TakeProfits: stats.TakeProfits, Invalidations: stats.Invalidations, TimeStops: stats.TimeStops, FillRate: stats.FillRate, MaxDeployed: stats.MaxDeployed, MaxDrawdown: stats.MaxDrawdown, FinalPnL: stats.FinalPnL}
		row.Score = nearMissLayerAuditScore(row)
		row.Verdict = nearMissLayerAuditVerdict(row)
		result.Rows = append(result.Rows, row)
	}
	sortNearMissLayerAuditRows(result.Rows)
	result.Summary = summarizeNearMissLayerAudit(result.Rows)
	return result, nil
}

func normalizeNearMissLayerAuditConfig(cfg config.Config, auditCfg NearMissLayerAuditConfig) NearMissLayerAuditConfig {
	if len(auditCfg.ReadinessThresholds) == 0 {
		auditCfg.ReadinessThresholds = []float64{0.35, 0.45, 0.55}
	}
	if len(auditCfg.InvalidationBuffers) == 0 {
		auditCfg.InvalidationBuffers = []float64{0.015, 0.030, 0.050, 0.080}
	}
	if len(auditCfg.TakeProfitPcts) == 0 {
		auditCfg.TakeProfitPcts = []float64{0.03, 0.05, 0.08, 0.12}
	}
	if len(auditCfg.TimeStopDays) == 0 {
		auditCfg.TimeStopDays = []int{0, 5, 10, 14}
	}
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	sort.Float64s(auditCfg.ReadinessThresholds)
	sort.Float64s(auditCfg.InvalidationBuffers)
	sort.Float64s(auditCfg.TakeProfitPcts)
	sort.Ints(auditCfg.TimeStopDays)
	return auditCfg
}

func newNearMissLayerState(key nearMissLayerKey) *nearMissLayerState {
	sim := Agent2Simulation{Enabled: true, Assets: map[string]AssetSimStats{key.symbol: {Symbol: key.symbol}}, Diagnostics: newAgent2Diagnostics([]string{key.symbol})}
	sim.Diagnostics.Notes = append(sim.Diagnostics.Notes, "Research-only forced near-miss layer audit; production plan/live behavior unchanged.")
	return &nearMissLayerState{key: key, sim: sim, orders: map[string][]simOrder{}, positions: map[string]*simPosition{key.symbol: {}}}
}

func nearMissLayerCandidateOK(c agent2.WatchCandidate) bool {
	if c.Actionable || c.Price <= 0 || !c.Support.Valid() {
		return false
	}
	for _, item := range c.EntryChecklist {
		if item.Pass || item.Severity != agent2.EntryCheckHard {
			continue
		}
		switch item.Name {
		case agent2.EntryCheckFallingKnife, agent2.EntryCheckFOMO, agent2.EntryCheckAssetFlowEntry:
			return false
		}
	}
	return true
}

func forcedNearMissAssetPlan(cfg config.Config, c agent2.WatchCandidate, buffer float64, index int) agent2.AssetPlan {
	budget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[c.Symbol] * cfg.Risk.MaxTotalDeploymentPerCycle
	if maxBudget := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment; maxBudget > 0 && budget > maxBudget {
		budget = maxBudget
	}
	prices := []float64{c.Support.High, c.Support.Mid(), c.Support.Low}
	layers := []agent2.Layer{}
	for i, fraction := range cfg.Execution.LayerDistribution {
		px := prices[minInt(i, len(prices)-1)]
		if px <= 0 {
			continue
		}
		notional := budget * fraction
		layers = append(layers, agent2.Layer{Index: i + 1, Fraction: fraction, Price: px, Notional: notional, Quantity: notional / px})
	}
	return agent2.AssetPlan{Symbol: c.Symbol, State: agent2.StateActiveLimit, DiscountZone: c.Support, Invalidation: c.Support.Low * (1 - buffer), RewardRisk: c.RewardRisk, RotationRank: c.RotationRank, RotationScore: c.RotationScore, AssetFlowBias: c.FlowBias, AssetFlowScore: c.FlowBullScore, Layers: layers, Reason: fmt.Sprintf("research-only forced near-miss layer from %s at index=%d", watchlistTrigger(c), index)}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func nearMissLayerAuditScore(row NearMissLayerAuditRow) float64 {
	return row.FinalPnL - math.Abs(row.MaxDrawdown*100)*2 - float64(row.Invalidations)*10 + float64(row.TakeProfits)*2 - float64(row.TimeStops)*0.5 + row.FillRate*2
}

func nearMissLayerAuditVerdict(row NearMissLayerAuditRow) string {
	if row.PlansCreated == 0 || row.OrdersPlaced == 0 {
		return NearMissLayerVerdictReject
	}
	if row.FinalPnL > 0 && row.Invalidations == 0 && row.MaxDrawdown > -0.12 {
		return NearMissLayerVerdictCandidate
	}
	if row.FinalPnL <= 0 && row.Invalidations > 0 {
		return NearMissLayerVerdictReject
	}
	return NearMissLayerVerdictWatch
}

func sortNearMissLayerAuditRows(rows []NearMissLayerAuditRow) {
	order := map[string]int{NearMissLayerVerdictCandidate: 0, NearMissLayerVerdictWatch: 1, NearMissLayerVerdictReject: 2}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Symbol != rows[j].Symbol {
			return rows[i].Symbol < rows[j].Symbol
		}
		if order[rows[i].Verdict] != order[rows[j].Verdict] {
			return order[rows[i].Verdict] < order[rows[j].Verdict]
		}
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].PlansCreated != rows[j].PlansCreated {
			return rows[i].PlansCreated > rows[j].PlansCreated
		}
		if rows[i].Trigger != rows[j].Trigger {
			return rows[i].Trigger < rows[j].Trigger
		}
		if rows[i].ReadinessThreshold != rows[j].ReadinessThreshold {
			return rows[i].ReadinessThreshold > rows[j].ReadinessThreshold
		}
		if rows[i].InvalidationBuffer != rows[j].InvalidationBuffer {
			return rows[i].InvalidationBuffer < rows[j].InvalidationBuffer
		}
		if rows[i].TakeProfitPct != rows[j].TakeProfitPct {
			return rows[i].TakeProfitPct < rows[j].TakeProfitPct
		}
		return rows[i].TimeStopDays < rows[j].TimeStopDays
	})
}

func summarizeNearMissLayerAudit(rows []NearMissLayerAuditRow) string {
	if len(rows) == 0 {
		return "Near-miss forced layer audit produced no rows."
	}
	candidates := 0
	watch := 0
	traded := 0
	bestSet := false
	var best NearMissLayerAuditRow
	for _, row := range rows {
		switch row.Verdict {
		case NearMissLayerVerdictCandidate:
			candidates++
		case NearMissLayerVerdictWatch:
			watch++
		}
		if row.OrdersPlaced == 0 {
			continue
		}
		traded++
		if !bestSet || row.Score > best.Score {
			best = row
			bestSet = true
		}
	}
	if !bestSet {
		return fmt.Sprintf("Near-miss forced layer audit rows=%d candidates=%d watch=%d traded=0; no near-miss candidate created forced layers.", len(rows), candidates, watch)
	}
	return fmt.Sprintf("Near-miss forced layer audit rows=%d candidates=%d watch=%d traded=%d best=%s %s threshold=%.2f buffer=%.3f tp=%.2f stop=%d pnl=%.2f", len(rows), candidates, watch, traded, best.Symbol, best.Trigger, best.ReadinessThreshold, best.InvalidationBuffer, best.TakeProfitPct, best.TimeStopDays, best.FinalPnL)
}
