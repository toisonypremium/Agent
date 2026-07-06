package backtest

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
)

const (
	TriggerBTCNotAllowed    = "BTC_NOT_ALLOWED"
	TriggerFlowNotConfirmed = "FLOW_NOT_CONFIRMED"
	TriggerDiscountNotReady = "DISCOUNT_NOT_READY"
	TriggerRRNotReady       = "RR_NOT_READY"
	TriggerRelativeWeak     = "RELATIVE_WEAK"
	TriggerRotationNotReady = "ROTATION_NOT_READY"
	TriggerFallingKnife     = "FALLING_KNIFE"
	TriggerFOMORisk         = "FOMO_RISK"
	TriggerReadyButNoPlan   = "READY_BUT_NO_PLAN"
	TriggerActiveLimit      = "ACTIVE_LIMIT"
)

type WatchlistTriggerAuditConfig struct {
	ReadinessThresholds []float64 `json:"readiness_thresholds"`
	HorizonDays         []int     `json:"horizon_days"`
	TargetSymbols       []string  `json:"target_symbols"`
	IncludeUnactionable bool      `json:"include_unactionable"`
}

type WatchlistTriggerAuditResult struct {
	Enabled bool                       `json:"enabled"`
	Rows    []WatchlistTriggerAuditRow `json:"rows"`
	Summary string                     `json:"summary"`
}

type WatchlistTriggerAuditRow struct {
	Symbol             string          `json:"symbol"`
	Trigger            string          `json:"trigger"`
	ReadinessThreshold float64         `json:"readiness_threshold"`
	Count              int             `json:"count"`
	AvgReturn          map[int]float64 `json:"avg_return"`
	WinRate            map[int]float64 `json:"win_rate"`
	WorstDrawdown      map[int]float64 `json:"worst_drawdown"`
	AvgReadiness       float64         `json:"avg_readiness"`
	Score              float64         `json:"score"`
	Verdict            string          `json:"verdict"`
}

type watchAuditKey struct {
	symbol    string
	trigger   string
	threshold float64
}

type watchAuditAcc struct {
	count          int
	readinessTotal float64
	returns        map[int]float64
	wins           map[int]int
	worstDD        map[int]float64
	initialized    map[int]bool
}

func RunWatchlistTriggerAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg WatchlistTriggerAuditConfig) (WatchlistTriggerAuditResult, error) {
	auditCfg = normalizeWatchlistTriggerAuditConfig(cfg, auditCfg)
	btc1d := btc["1d"]
	maxH := maxHorizon(auditCfg.HorizonDays)
	warmup := 60
	need := warmup + maxH + 1
	if len(btc1d) < need {
		return WatchlistTriggerAuditResult{}, fmt.Errorf("not enough BTC 1d candles for watchlist trigger audit; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, assets) - 1
	if lastIndex < need {
		return WatchlistTriggerAuditResult{}, fmt.Errorf("not enough aligned asset candles for watchlist trigger audit; need %d got %d", need, lastIndex+1)
	}

	acc := map[watchAuditKey]*watchAuditAcc{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	targets := targetSet(auditCfg.TargetSymbols)
	for i := warmup; i+maxH <= lastIndex; i++ {
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
		for _, candidate := range plan.Watchlist.Candidates {
			if !auditCfg.IncludeUnactionable && !candidate.Actionable {
				continue
			}
			if !targets[candidate.Symbol] {
				continue
			}
			candles := assets[candidate.Symbol]
			if len(candles) <= i+maxH || candles[i].Close <= 0 {
				continue
			}
			for _, threshold := range auditCfg.ReadinessThresholds {
				if candidate.ReadinessScore < threshold {
					continue
				}
				key := watchAuditKey{symbol: candidate.Symbol, trigger: watchlistTrigger(candidate), threshold: threshold}
				a := acc[key]
				if a == nil {
					a = newWatchAuditAcc(auditCfg.HorizonDays)
					acc[key] = a
				}
				entry := candles[i].Close
				a.count++
				a.readinessTotal += candidate.ReadinessScore
				for _, h := range auditCfg.HorizonDays {
					future := candles[i+h]
					ret := (future.Close - entry) / entry
					dd := worstDrawdown(candles[i+1:i+h+1], entry)
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
		}
	}

	result := WatchlistTriggerAuditResult{Enabled: true}
	for key, a := range acc {
		row := WatchlistTriggerAuditRow{Symbol: key.symbol, Trigger: key.trigger, ReadinessThreshold: key.threshold, Count: a.count, AvgReturn: map[int]float64{}, WinRate: map[int]float64{}, WorstDrawdown: map[int]float64{}}
		if a.count > 0 {
			row.AvgReadiness = a.readinessTotal / float64(a.count)
		}
		for _, h := range auditCfg.HorizonDays {
			if a.count > 0 {
				row.AvgReturn[h] = a.returns[h] / float64(a.count)
				row.WinRate[h] = float64(a.wins[h]) / float64(a.count)
				row.WorstDrawdown[h] = a.worstDD[h]
			}
		}
		row.Score = watchlistTriggerAuditScore(row, auditCfg.HorizonDays)
		row.Verdict = watchlistTriggerAuditVerdict(row, auditCfg.HorizonDays)
		result.Rows = append(result.Rows, row)
	}
	sortWatchlistTriggerAuditRows(result.Rows)
	result.Summary = summarizeWatchlistTriggerAudit(result.Rows)
	return result, nil
}

func normalizeWatchlistTriggerAuditConfig(cfg config.Config, auditCfg WatchlistTriggerAuditConfig) WatchlistTriggerAuditConfig {
	if len(auditCfg.ReadinessThresholds) == 0 {
		auditCfg.ReadinessThresholds = []float64{0.60, 0.70, 0.80}
	}
	if len(auditCfg.HorizonDays) == 0 {
		auditCfg.HorizonDays = []int{3, 7, 14}
	}
	outH := auditCfg.HorizonDays[:0]
	seenH := map[int]bool{}
	for _, h := range auditCfg.HorizonDays {
		if h > 0 && !seenH[h] {
			outH = append(outH, h)
			seenH[h] = true
		}
	}
	sort.Ints(outH)
	auditCfg.HorizonDays = outH
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func watchlistTrigger(c agent2.WatchCandidate) string {
	joined := strings.ToLower(strings.Join(c.Missing, " ") + " " + c.BlockReason)
	switch {
	case strings.Contains(joined, "falling knife"):
		return TriggerFallingKnife
	case strings.Contains(joined, "fomo"):
		return TriggerFOMORisk
	case strings.Contains(joined, "relative"):
		return TriggerRelativeWeak
	case strings.Contains(joined, "btc permission") || strings.Contains(joined, "btc risk"):
		return TriggerBTCNotAllowed
	case strings.Contains(joined, "rotation"):
		return TriggerRotationNotReady
	case strings.Contains(joined, "flow") || strings.Contains(joined, "reclaim") || strings.Contains(joined, "absorption"):
		return TriggerFlowNotConfirmed
	case strings.Contains(joined, "discount"):
		return TriggerDiscountNotReady
	case strings.Contains(joined, "reward/risk"):
		return TriggerRRNotReady
	case c.State == agent2.StateActiveLimit:
		return TriggerActiveLimit
	default:
		return TriggerReadyButNoPlan
	}
}

func newWatchAuditAcc(horizons []int) *watchAuditAcc {
	a := &watchAuditAcc{returns: map[int]float64{}, wins: map[int]int{}, worstDD: map[int]float64{}, initialized: map[int]bool{}}
	for _, h := range horizons {
		a.returns[h] = 0
		a.wins[h] = 0
		a.worstDD[h] = 0
		a.initialized[h] = false
	}
	return a
}

func watchlistTriggerAuditScore(row WatchlistTriggerAuditRow, horizons []int) float64 {
	if len(horizons) == 0 {
		return 0
	}
	lastH := horizons[len(horizons)-1]
	return row.AvgReturn[lastH]*100 + row.WinRate[lastH]*5 - math.Abs(row.WorstDrawdown[lastH]*100)*1.5
}

func watchlistTriggerAuditVerdict(row WatchlistTriggerAuditRow, horizons []int) string {
	if row.Count < 5 || len(horizons) == 0 {
		return "REJECT"
	}
	lastH := horizons[len(horizons)-1]
	if row.AvgReturn[lastH] <= 0 && row.WorstDrawdown[lastH] < -0.10 {
		return "REJECT"
	}
	if row.AvgReturn[lastH] > 0 && row.WinRate[lastH] >= 0.50 && row.WorstDrawdown[lastH] > -0.12 {
		return "CANDIDATE"
	}
	return "WATCH"
}

func sortWatchlistTriggerAuditRows(rows []WatchlistTriggerAuditRow) {
	order := map[string]int{"CANDIDATE": 0, "WATCH": 1, "REJECT": 2}
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
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].ReadinessThreshold != rows[j].ReadinessThreshold {
			return rows[i].ReadinessThreshold > rows[j].ReadinessThreshold
		}
		return rows[i].Trigger < rows[j].Trigger
	})
}

func summarizeWatchlistTriggerAudit(rows []WatchlistTriggerAuditRow) string {
	if len(rows) == 0 {
		return "Watchlist trigger audit produced no rows; no candidate met readiness thresholds."
	}
	candidates := 0
	watch := 0
	for _, row := range rows {
		switch row.Verdict {
		case "CANDIDATE":
			candidates++
		case "WATCH":
			watch++
		}
	}
	best := rows[0]
	return fmt.Sprintf("Watchlist trigger audit rows=%d candidates=%d watch=%d best=%s %s threshold=%.2f count=%d score=%.2f", len(rows), candidates, watch, best.Symbol, best.Trigger, best.ReadinessThreshold, best.Count, best.Score)
}

func targetSet(symbols []string) map[string]bool {
	out := map[string]bool{}
	for _, sym := range symbols {
		out[sym] = true
	}
	return out
}
