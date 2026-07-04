package backtest

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

type LayerAuditConfig struct {
	InvalidationBuffers   []float64 `json:"invalidation_buffers"`
	LayerDepthMultipliers []float64 `json:"layer_depth_multipliers"`
	TargetSymbols         []string  `json:"target_symbols"`
}

type LayerAuditResult struct {
	Enabled bool            `json:"enabled"`
	Rows    []LayerAuditRow `json:"rows"`
	Summary string          `json:"summary"`
}

type LayerAuditRow struct {
	Symbol               string  `json:"symbol"`
	InvalidationBuffer   float64 `json:"invalidation_buffer"`
	LayerDepthMultiplier float64 `json:"layer_depth_multiplier"`
	PlansCreated         int     `json:"plans_created"`
	OrdersPlaced         int     `json:"orders_placed"`
	OrdersFilled         int     `json:"orders_filled"`
	OrdersExpired        int     `json:"orders_expired"`
	Invalidations        int     `json:"invalidations"`
	FillRate             float64 `json:"fill_rate"`
	MaxDeployed          float64 `json:"max_deployed"`
	MaxDrawdown          float64 `json:"max_drawdown"`
	FinalPnL             float64 `json:"final_pnl"`
	Score                float64 `json:"score"`
	Verdict              string  `json:"verdict"`
}

func RunLayerAudit(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, auditCfg LayerAuditConfig) (LayerAuditResult, error) {
	auditCfg = normalizeLayerAuditConfig(cfg, auditCfg)
	if len(auditCfg.TargetSymbols) == 0 {
		return LayerAuditResult{}, fmt.Errorf("no target symbols for layer audit")
	}
	result := LayerAuditResult{Enabled: true}
	for _, sym := range auditCfg.TargetSymbols {
		if len(assets[sym]) == 0 {
			continue
		}
		for _, buffer := range auditCfg.InvalidationBuffers {
			for _, depth := range auditCfg.LayerDepthMultipliers {
				cfg2 := cfg
				cfg2.Data.Symbols.Assets = []string{sym}
				sim, err := RunAgent2SimulationWithOverrides(cfg2, btc, assets, SimulationOverrides{InvalidationBuffer: buffer, LayerDepthMultiplier: depth, TargetSymbols: map[string]bool{sym: true}})
				if err != nil {
					continue
				}
				stats := sim.Assets[sym]
				row := LayerAuditRow{Symbol: sym, InvalidationBuffer: buffer, LayerDepthMultiplier: depth, PlansCreated: stats.PlansCreated, OrdersPlaced: stats.OrdersPlaced, OrdersFilled: stats.OrdersFilled, OrdersExpired: stats.OrdersExpired, Invalidations: stats.Invalidations, FillRate: stats.FillRate, MaxDeployed: stats.MaxDeployed, MaxDrawdown: stats.MaxDrawdown, FinalPnL: stats.FinalPnL}
				row.Score = layerAuditScore(row)
				row.Verdict = layerAuditVerdict(row)
				result.Rows = append(result.Rows, row)
			}
		}
	}
	sortLayerAuditRows(result.Rows)
	result.Summary = summarizeLayerAudit(result.Rows)
	return result, nil
}

func normalizeLayerAuditConfig(cfg config.Config, auditCfg LayerAuditConfig) LayerAuditConfig {
	if len(auditCfg.InvalidationBuffers) == 0 {
		auditCfg.InvalidationBuffers = []float64{0.015, 0.030, 0.050, 0.080}
	}
	if len(auditCfg.LayerDepthMultipliers) == 0 {
		auditCfg.LayerDepthMultipliers = []float64{1.00, 1.25, 1.50}
	}
	if len(auditCfg.TargetSymbols) == 0 {
		auditCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	return auditCfg
}

func layerAuditScore(row LayerAuditRow) float64 {
	return row.FinalPnL - math.Abs(row.MaxDrawdown*100)*2 - float64(row.Invalidations)*10 + row.FillRate*2
}

func layerAuditVerdict(row LayerAuditRow) string {
	if row.PlansCreated == 0 || row.OrdersPlaced == 0 {
		return "REJECT"
	}
	if row.Invalidations > 0 {
		return "REJECT"
	}
	if row.FinalPnL > 0 && row.MaxDrawdown > -0.12 {
		return "CANDIDATE"
	}
	return "WATCH"
}

func sortLayerAuditRows(rows []LayerAuditRow) {
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
		if rows[i].MaxDrawdown != rows[j].MaxDrawdown {
			return rows[i].MaxDrawdown > rows[j].MaxDrawdown
		}
		if rows[i].InvalidationBuffer != rows[j].InvalidationBuffer {
			return rows[i].InvalidationBuffer < rows[j].InvalidationBuffer
		}
		return rows[i].LayerDepthMultiplier < rows[j].LayerDepthMultiplier
	})
}

func summarizeLayerAudit(rows []LayerAuditRow) string {
	if len(rows) == 0 {
		return "Layer audit skipped or no valid asset rows."
	}
	candidates := 0
	watch := 0
	traded := 0
	var best LayerAuditRow
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
		return fmt.Sprintf("Layer audit rows=%d candidates=%d watch=%d traded=0; no asset produced layer plans.", len(rows), candidates, watch)
	}
	return fmt.Sprintf("Layer audit rows=%d candidates=%d watch=%d traded=%d best_traded=%s buffer=%.3f depth=%.2f invalidations=%d pnl=%.2f", len(rows), candidates, watch, traded, best.Symbol, best.InvalidationBuffer, best.LayerDepthMultiplier, best.Invalidations, best.FinalPnL)
}
