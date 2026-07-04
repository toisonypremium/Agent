package backtest

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	DecisionRowBTCPermission  = "BTC_PERMISSION"
	DecisionRowAssetWatchlist = "ASSET_WATCHLIST"

	LabelNoTradeCorrect         = "NO_TRADE_CORRECT"
	LabelWatchCorrect           = "WATCH_CORRECT"
	LabelArmedCorrect           = "ARMED_CORRECT"
	LabelAllowedCorrect         = "ALLOWED_CORRECT"
	LabelFalseAllowed           = "FALSE_ALLOWED"
	LabelMissedOpportunity      = "MISSED_OPPORTUNITY"
	LabelRiskAvoided            = "RISK_AVOIDED"
	LabelFlowValidBullish       = "FLOW_VALID_BULLISH"
	LabelFlowFalseBullish       = "FLOW_FALSE_BULLISH"
	LabelFlowValidBearish       = "FLOW_VALID_BEARISH"
	LabelFlowFalseBearish       = "FLOW_FALSE_BEARISH"
	LabelFlowNeutralCorrect     = "FLOW_NEUTRAL_CORRECT"
	LabelFlowNeutralMissedMove  = "FLOW_NEUTRAL_MISSED_MOVE"
	LabelActionableWatchCorrect = "ACTIONABLE_WATCH_CORRECT"
	LabelBlockedCorrect         = "BLOCKED_CORRECT"
	LabelFalseWatch             = "FALSE_WATCH"
	LabelMissedEntry            = "MISSED_ENTRY"
)

type TrainingDatasetConfig struct {
	MinWindow1D   int      `json:"min_window_1d"`
	HorizonDays   []int    `json:"horizon_days"`
	TargetSymbols []string `json:"target_symbols"`
	MaxRows       int      `json:"max_rows"`
}

type TrainingDatasetResult struct {
	Enabled   bool   `json:"enabled"`
	Rows      int    `json:"rows"`
	JSONLPath string `json:"jsonl_path"`
	CSVPath   string `json:"csv_path"`
	Summary   string `json:"summary"`
}

type DecisionDatasetRow struct {
	Timestamp        string            `json:"timestamp"`
	Symbol           string            `json:"symbol"`
	RowType          string            `json:"row_type"`
	BTCPrice         float64           `json:"btc_price"`
	MarketRegime     string            `json:"market_regime"`
	TrendScore       float64           `json:"trend_score"`
	RiskLevel        agent1.Risk       `json:"risk_level"`
	FallingKnifeRisk agent1.Risk       `json:"falling_knife_risk"`
	FomoRisk         agent1.Risk       `json:"fomo_risk"`
	FlowBias         flow.Bias         `json:"flow_bias"`
	FlowScore        float64           `json:"flow_score"`
	FlowDailyBias    flow.Bias         `json:"flow_daily_bias"`
	FlowLabel        string            `json:"flow_label,omitempty"`
	ActionPermission agent1.Permission `json:"action_permission"`
	WatchTier        string            `json:"watch_tier,omitempty"`
	WatchActionable  bool              `json:"watch_actionable,omitempty"`
	ReadinessScore   float64           `json:"readiness_score,omitempty"`
	ChecklistSummary string            `json:"checklist_summary,omitempty"`
	Missing          []string          `json:"missing,omitempty"`
	NextTrigger      string            `json:"next_trigger,omitempty"`
	TopBlockers      []string          `json:"top_blockers,omitempty"`
	ForwardReturn    map[int]float64   `json:"forward_return"`
	ForwardDrawdown  map[int]float64   `json:"forward_drawdown"`
	Label            string            `json:"label"`
	Explanation      string            `json:"explanation"`
}

func BuildTrainingDataset(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, outDir string, dsCfg TrainingDatasetConfig) (TrainingDatasetResult, error) {
	dsCfg = normalizeTrainingDatasetConfig(cfg, dsCfg)
	if outDir == "" {
		outDir = filepath.Join("data", "training")
	}
	btc1d := btc["1d"]
	maxH := maxHorizon(dsCfg.HorizonDays)
	need := dsCfg.MinWindow1D + maxH + 1
	if len(btc1d) < need {
		return TrainingDatasetResult{}, fmt.Errorf("not enough BTC 1d candles for training dataset; need %d got %d", need, len(btc1d))
	}
	lastIndex := minLen(btc1d, filterAssets(assets, dsCfg.TargetSymbols)) - 1
	if lastIndex < dsCfg.MinWindow1D+maxH {
		return TrainingDatasetResult{}, fmt.Errorf("not enough aligned candles for training dataset; need %d got %d", need, lastIndex+1)
	}

	rows := []DecisionDatasetRow{}
	neutralFG := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	stop := false
	for i := dsCfg.MinWindow1D; i+maxH <= lastIndex && !stop; i++ {
		btcWindow := map[string][]market.Candle{"1d": btc1d[:i+1], "4h": btc1d[:i+1], "1w": btc1d[:i+1]}
		analysis, err := agent1.Analyze(cfg, btcWindow, neutralFG)
		if err != nil {
			continue
		}
		assetWindows := map[string][]market.Candle{}
		for _, sym := range dsCfg.TargetSymbols {
			if len(assets[sym]) > i {
				assetWindows[sym] = assets[sym][:i+1]
			}
		}
		benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d[:i+1], "BTCUSDT": btc1d[:i+1]}
		plan := agent2.BuildPlanWithBenchmarks(cfg, analysis, assetWindows, benchmarks)

		btcRow := buildBTCDatasetRow(cfg, analysis, btc1d, i, dsCfg.HorizonDays)
		rows = append(rows, btcRow)
		if dsCfg.MaxRows > 0 && len(rows) >= dsCfg.MaxRows {
			break
		}

		candidatesBySymbol := map[string]agent2.WatchCandidate{}
		for _, c := range plan.Watchlist.Candidates {
			candidatesBySymbol[c.Symbol] = c
		}
		for _, sym := range dsCfg.TargetSymbols {
			candidate, ok := candidatesBySymbol[sym]
			if !ok || len(assets[sym]) <= i+maxH || assets[sym][i].Close <= 0 {
				continue
			}
			rows = append(rows, buildAssetDatasetRow(analysis, candidate, assets[sym], i, dsCfg.HorizonDays))
			if dsCfg.MaxRows > 0 && len(rows) >= dsCfg.MaxRows {
				stop = true
				break
			}
		}
	}

	jsonlPath := filepath.Join(outDir, "decision_dataset.jsonl")
	csvPath := filepath.Join(outDir, "decision_dataset.csv")
	if err := writeDecisionDatasetFiles(outDir, jsonlPath, csvPath, rows); err != nil {
		return TrainingDatasetResult{}, err
	}
	result := TrainingDatasetResult{Enabled: true, Rows: len(rows), JSONLPath: jsonlPath, CSVPath: csvPath}
	result.Summary = fmt.Sprintf("Training dataset rows=%d jsonl=%s csv=%s", result.Rows, result.JSONLPath, result.CSVPath)
	return result, nil
}

func normalizeTrainingDatasetConfig(cfg config.Config, dsCfg TrainingDatasetConfig) TrainingDatasetConfig {
	if dsCfg.MinWindow1D <= 0 {
		dsCfg.MinWindow1D = 60
	}
	if len(dsCfg.HorizonDays) == 0 {
		dsCfg.HorizonDays = []int{3, 7, 14}
	}
	if len(dsCfg.TargetSymbols) == 0 {
		dsCfg.TargetSymbols = append([]string(nil), cfg.Data.Symbols.Assets...)
	}
	seen := map[int]bool{}
	out := []int{}
	for _, h := range dsCfg.HorizonDays {
		if h > 0 && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}
	if !seen[7] {
		out = append(out, 7)
	}
	sort.Ints(out)
	dsCfg.HorizonDays = out
	return dsCfg
}

func filterAssets(assets map[string][]market.Candle, symbols []string) map[string][]market.Candle {
	out := map[string][]market.Candle{}
	for _, sym := range symbols {
		if len(assets[sym]) > 0 {
			out[sym] = assets[sym]
		}
	}
	return out
}

func buildBTCDatasetRow(cfg config.Config, analysis agent1.MarketAnalysis, btc1d []market.Candle, index int, horizons []int) DecisionDatasetRow {
	returns := forwardReturnMap(btc1d, index, horizons)
	drawdowns := forwardDrawdownMap(btc1d, index, horizons)
	label := btcDecisionLabel(analysis.ActionPermission, returns[7], drawdowns[7])
	row := baseDecisionDatasetRow(cfg.Data.Symbols.BTC, DecisionRowBTCPermission, analysis, btc1d[index], returns, drawdowns)
	row.TopBlockers = btcPermissionBlockers(analysis)
	row.FlowLabel = flowDecisionLabel(analysis.Flow.Bias, returns[7], drawdowns[7])
	row.Label = label
	row.Explanation = btcDatasetExplanation(row)
	return row
}

func buildAssetDatasetRow(analysis agent1.MarketAnalysis, candidate agent2.WatchCandidate, candles []market.Candle, index int, horizons []int) DecisionDatasetRow {
	returns := forwardReturnMap(candles, index, horizons)
	drawdowns := forwardDrawdownMap(candles, index, horizons)
	row := baseDecisionDatasetRow(candidate.Symbol, DecisionRowAssetWatchlist, analysis, candles[index], returns, drawdowns)
	row.WatchTier = candidate.Tier
	row.WatchActionable = candidate.Actionable
	row.ReadinessScore = candidate.ReadinessScore
	row.ChecklistSummary = agent2.ChecklistSummary(candidate.EntryChecklist)
	row.Missing = append([]string(nil), candidate.Missing...)
	row.NextTrigger = candidate.NextTrigger
	row.TopBlockers = append([]string(nil), candidate.NoiseFlags...)
	if len(row.TopBlockers) == 0 {
		row.TopBlockers = append([]string(nil), candidate.Missing...)
	}
	row.Label = assetWatchlistLabel(candidate, returns[7], drawdowns[7])
	row.Explanation = assetDatasetExplanation(row)
	return row
}

func baseDecisionDatasetRow(symbol, rowType string, analysis agent1.MarketAnalysis, candle market.Candle, returns, drawdowns map[int]float64) DecisionDatasetRow {
	return DecisionDatasetRow{
		Timestamp:        eventTime(candle.CloseTime),
		Symbol:           symbol,
		RowType:          rowType,
		BTCPrice:         analysis.BTCPrice,
		MarketRegime:     analysis.MarketRegime,
		TrendScore:       analysis.TrendScore,
		RiskLevel:        analysis.RiskLevel,
		FallingKnifeRisk: analysis.FallingKnifeRisk,
		FomoRisk:         analysis.FomoRisk,
		FlowBias:         analysis.Flow.Bias,
		FlowScore:        analysis.Flow.Score,
		FlowDailyBias:    analysis.Flow.Daily.FlowBias,
		ActionPermission: analysis.ActionPermission,
		ForwardReturn:    returns,
		ForwardDrawdown:  drawdowns,
	}
}

func forwardReturnMap(candles []market.Candle, index int, horizons []int) map[int]float64 {
	out := map[int]float64{}
	entry := candles[index].Close
	for _, h := range horizons {
		if entry > 0 && index+h < len(candles) {
			out[h] = (candles[index+h].Close - entry) / entry
		}
	}
	return out
}

func forwardDrawdownMap(candles []market.Candle, index int, horizons []int) map[int]float64 {
	out := map[int]float64{}
	entry := candles[index].Close
	for _, h := range horizons {
		if entry > 0 && index+h < len(candles) {
			out[h] = worstDrawdown(candles[index+1:index+h+1], entry)
		}
	}
	return out
}

func btcDecisionLabel(permission agent1.Permission, ret7 float64, dd7 float64) string {
	switch permission {
	case agent1.Allowed:
		if ret7 < 0 || dd7 <= -0.08 {
			return LabelFalseAllowed
		}
		if ret7 > 0 && dd7 > -0.08 {
			return LabelAllowedCorrect
		}
	case agent1.Armed:
		if ret7 > 0.03 && dd7 > -0.08 {
			return LabelArmedCorrect
		}
	case agent1.Watch:
		if ret7 > 0.04 && dd7 > -0.06 {
			return LabelMissedOpportunity
		}
	case agent1.NoTrade:
		if dd7 <= -0.08 {
			return LabelRiskAvoided
		}
		if ret7 < 0 {
			return LabelNoTradeCorrect
		}
		if ret7 > 0.05 && dd7 > -0.05 {
			return LabelMissedOpportunity
		}
	}
	return LabelWatchCorrect
}

func flowDecisionLabel(bias flow.Bias, ret7 float64, dd7 float64) string {
	switch bias {
	case flow.BiasAccumulation, flow.BiasBearTrap:
		if ret7 > 0 && dd7 > -0.08 {
			return LabelFlowValidBullish
		}
		return LabelFlowFalseBullish
	case flow.BiasDistribution, flow.BiasBullTrap:
		if ret7 < 0 || dd7 <= -0.08 {
			return LabelFlowValidBearish
		}
		if ret7 > 0.04 && dd7 > -0.06 {
			return LabelFlowFalseBearish
		}
	case flow.BiasNeutral:
		if math.Abs(ret7) < 0.03 {
			return LabelFlowNeutralCorrect
		}
		if ret7 > 0.05 && dd7 > -0.06 {
			return LabelFlowNeutralMissedMove
		}
	}
	return LabelFlowNeutralCorrect
}

func assetWatchlistLabel(candidate agent2.WatchCandidate, ret7 float64, dd7 float64) string {
	if candidate.Actionable && ret7 > 0 && dd7 > -0.10 {
		return LabelActionableWatchCorrect
	}
	if candidate.Actionable && (ret7 < 0 || dd7 <= -0.12) {
		return LabelFalseWatch
	}
	if !candidate.Actionable && candidate.Tier == agent2.WatchTierBlocked && (ret7 < 0 || dd7 <= -0.12) {
		return LabelBlockedCorrect
	}
	if !candidate.Actionable && ret7 > 0.08 && dd7 > -0.08 {
		return LabelMissedEntry
	}
	return LabelWatchCorrect
}

func btcDatasetExplanation(row DecisionDatasetRow) string {
	blockers := "none"
	if len(row.TopBlockers) > 0 {
		blockers = strings.Join(row.TopBlockers, ", ")
	}
	return fmt.Sprintf("BTC permission %s because %s; 7D return %.2f%%, drawdown %.2f%%; flow label %s; label %s.", row.ActionPermission, blockers, row.ForwardReturn[7]*100, row.ForwardDrawdown[7]*100, row.FlowLabel, row.Label)
}

func assetDatasetExplanation(row DecisionDatasetRow) string {
	return fmt.Sprintf("%s watchlist %s readiness %.2f; checklist %s; next trigger: %s; 7D return %.2f%%, drawdown %.2f%%; label %s.", row.Symbol, row.WatchTier, row.ReadinessScore, row.ChecklistSummary, row.NextTrigger, row.ForwardReturn[7]*100, row.ForwardDrawdown[7]*100, row.Label)
}

func writeDecisionDatasetFiles(outDir, jsonlPath, csvPath string, rows []DecisionDatasetRow) error {
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return err
	}
	var jsonl bytes.Buffer
	enc := json.NewEncoder(&jsonl)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	if err := os.WriteFile(jsonlPath, jsonl.Bytes(), 0600); err != nil {
		return err
	}
	csvData, err := decisionDatasetCSV(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(csvPath, csvData, 0600)
}

func decisionDatasetCSV(rows []DecisionDatasetRow) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"timestamp", "row_type", "symbol", "btc_price", "market_regime", "trend_score", "risk_level", "falling_knife_risk", "fomo_risk", "flow_bias", "flow_score", "action_permission", "watch_tier", "watch_actionable", "readiness_score", "top_blockers", "checklist_summary", "forward_return_3d", "forward_return_7d", "forward_return_14d", "forward_drawdown_7d", "label", "explanation"}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := []string{
			row.Timestamp,
			row.RowType,
			row.Symbol,
			fmt.Sprintf("%.8f", row.BTCPrice),
			row.MarketRegime,
			fmt.Sprintf("%.4f", row.TrendScore),
			string(row.RiskLevel),
			string(row.FallingKnifeRisk),
			string(row.FomoRisk),
			string(row.FlowBias),
			fmt.Sprintf("%.4f", row.FlowScore),
			string(row.ActionPermission),
			row.WatchTier,
			fmt.Sprintf("%t", row.WatchActionable),
			fmt.Sprintf("%.4f", row.ReadinessScore),
			strings.Join(row.TopBlockers, ";"),
			row.ChecklistSummary,
			fmt.Sprintf("%.8f", row.ForwardReturn[3]),
			fmt.Sprintf("%.8f", row.ForwardReturn[7]),
			fmt.Sprintf("%.8f", row.ForwardReturn[14]),
			fmt.Sprintf("%.8f", row.ForwardDrawdown[7]),
			row.Label,
			row.Explanation,
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
