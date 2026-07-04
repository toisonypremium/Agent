package backtest

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

type ExitAuditConfig struct {
	TakeProfitPcts       []float64 `json:"take_profit_pcts"`
	TimeStopDays         []int     `json:"time_stop_days"`
	InvalidationBuffer   float64   `json:"invalidation_buffer"`
	LayerDepthMultiplier float64   `json:"layer_depth_multiplier"`
	TargetSymbols        []string  `json:"target_symbols"`
}

type ExitAuditResult struct {
	Enabled bool           `json:"enabled"`
	Rows    []ExitAuditRow `json:"rows"`
	Summary string         `json:"summary"`
}

type ExitAuditRow struct {
	Symbol        string  `json:"symbol"`
	TakeProfitPct float64 `json:"take_profit_pct"`
	TimeStopDays  int     `json:"time_stop_days"`
	PlansCreated  int     `json:"plans_created"`
	OrdersPlaced  int     `json:"orders_placed"`
	OrdersFilled  int     `json:"orders_filled"`
	OrdersExpired int     `json:"orders_expired"`
	Invalidations int     `json:"invalidations"`
	TakeProfits   int     `json:"take_profits"`
	TimeStops     int     `json:"time_stops"`
	FillRate      float64 `json:"fill_rate"`
	MaxDeployed   float64 `json:"max_deployed"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	FinalPnL      float64 `json:"final_pnl"`
	Score         float64 `json:"score"`
	Verdict       string  `json:"verdict"`
}

func RunExitAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg ExitAuditConfig) (ExitAuditResult, error) {
	auditCfg = normalizeExitAuditConfig(cfg, auditCfg)
	if len(auditCfg.TargetSymbols) == 0 {
		return ExitAuditResult{}, fmt.Errorf("no target symbols for exit audit")
	}
	result := ExitAuditResult{Enabled: true}
	for _, sym := range auditCfg.TargetSymbols {
		if len(assets[sym]) == 0 {
			continue
		}
		for _, tp := range auditCfg.TakeProfitPcts {
			for _, timeStop := range auditCfg.TimeStopDays {
				cfg2 := cfg
				cfg2.Data.Symbols.Assets = []string{sym}
				sim, err := RunAgent2SimulationWithOverrides(cfg2, btc, assets, SimulationOverrides{InvalidationBuffer: auditCfg.InvalidationBuffer, LayerDepthMultiplier: auditCfg.LayerDepthMultiplier, TargetSymbols: map[string]bool{sym: true}, TakeProfitPct: tp, TimeStopDays: timeStop})
				if err != nil {
					continue
				}
				stats := sim.Assets[sym]
				row := ExitAuditRow{Symbol: sym, TakeProfitPct: tp, TimeStopDays: timeStop, PlansCreated: stats.PlansCreated, OrdersPlaced: stats.OrdersPlaced, OrdersFilled: stats.OrdersFilled, OrdersExpired: stats.OrdersExpired, Invalidations: stats.Invalidations, TakeProfits: stats.TakeProfits, TimeStops: stats.TimeStops, FillRate: stats.FillRate, MaxDeployed: stats.MaxDeployed, MaxDrawdown: stats.MaxDrawdown, FinalPnL: stats.FinalPnL}
				row.Score = exitAuditScore(row)
				row.Verdict = exitAuditVerdict(row)
				result.Rows = append(result.Rows, row)
			}
		}
	}
	sortExitAuditRows(result.Rows)
	result.Summary = summarizeExitAudit(result.Rows)
	return result, nil
}

func normalizeExitAuditConfig(cfg config.Config, auditCfg ExitAuditConfig) ExitAuditConfig {
	if len(auditCfg.TakeProfitPcts) == 0 {
		auditCfg.TakeProfitPcts = []float64{0.03, 0.05, 0.08, 0.12}
	}
	if len(auditCfg.TimeStopDays) == 0 {
		auditCfg.TimeStopDays = []int{0, 3, 5, 7, 14}
	}
	if auditCfg.InvalidationBuffer <= 0 {
		auditCfg.InvalidationBuffer = 0.015
	}
	if auditCfg.LayerDepthMultiplier <= 0 {
		auditCfg.LayerDepthMultiplier = 1.0
	}
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func exitAuditScore(row ExitAuditRow) float64 {
	return row.FinalPnL + float64(row.TakeProfits)*5 - float64(row.Invalidations)*12 - math.Abs(row.MaxDrawdown*100)*2 + row.FillRate*2
}

func exitAuditVerdict(row ExitAuditRow) string {
	if row.PlansCreated == 0 || row.OrdersPlaced == 0 {
		return "REJECT"
	}
	if row.Invalidations > row.TakeProfits {
		return "REJECT"
	}
	if row.FinalPnL > 0 && row.Invalidations == 0 && row.TakeProfits > 0 && row.MaxDrawdown > -0.12 {
		return "CANDIDATE"
	}
	if row.TakeProfits > 0 || row.TimeStops > 0 || row.Invalidations == 0 {
		return "WATCH"
	}
	return "REJECT"
}

func sortExitAuditRows(rows []ExitAuditRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Symbol != rows[j].Symbol {
			return rows[i].Symbol < rows[j].Symbol
		}
		if rows[i].Invalidations != rows[j].Invalidations {
			return rows[i].Invalidations < rows[j].Invalidations
		}
		if rows[i].FinalPnL != rows[j].FinalPnL {
			return rows[i].FinalPnL > rows[j].FinalPnL
		}
		if rows[i].TakeProfits != rows[j].TakeProfits {
			return rows[i].TakeProfits > rows[j].TakeProfits
		}
		if rows[i].MaxDrawdown != rows[j].MaxDrawdown {
			return rows[i].MaxDrawdown > rows[j].MaxDrawdown
		}
		if rows[i].TakeProfitPct != rows[j].TakeProfitPct {
			return rows[i].TakeProfitPct < rows[j].TakeProfitPct
		}
		return rows[i].TimeStopDays < rows[j].TimeStopDays
	})
}

func summarizeExitAudit(rows []ExitAuditRow) string {
	if len(rows) == 0 {
		return "Exit audit skipped or no valid asset rows."
	}
	candidates := 0
	watch := 0
	traded := 0
	var best ExitAuditRow
	bestSet := false
	for _, row := range rows {
		switch row.Verdict {
		case "CANDIDATE":
			candidates++
		case "WATCH":
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
		return fmt.Sprintf("Exit audit rows=%d candidates=%d watch=%d traded=0; no asset produced layer plans.", len(rows), candidates, watch)
	}
	return fmt.Sprintf("Exit audit rows=%d candidates=%d watch=%d traded=%d best_traded=%s tp=%.2f%% time_stop=%d invalidations=%d take_profits=%d pnl=%.2f", len(rows), candidates, watch, traded, best.Symbol, best.TakeProfitPct*100, best.TimeStopDays, best.Invalidations, best.TakeProfits, best.FinalPnL)
}
