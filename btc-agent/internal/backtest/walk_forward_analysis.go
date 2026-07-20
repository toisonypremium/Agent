package backtest

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
	"fmt"
)

type WalkForwardAnalysisReport struct {
	Enabled bool                       `json:"enabled"`
	Splits  []WalkForwardAnalysisSplit `json:"splits"`
	Summary string                     `json:"summary"`
}
type WalkForwardAnalysisSplit struct {
	SplitIndex, TrainStart, TrainEnd, EvalStart, EvalEnd, Embargo int
	Train, Eval                                                   CoreSignalStats
}
type CoreSignalStats struct {
	Samples     int                       `json:"samples"`
	Permissions map[agent1.Permission]int `json:"permissions"`
	Biases      map[flow.Bias]int         `json:"biases"`
}

func RunWalkForwardAnalysis(cfg config.Config, btc map[string][]market.Candle, splitCount int, trainPct float64, embargo int) (WalkForwardAnalysisReport, error) {
	daily := btc["1d"]
	if splitCount <= 0 || trainPct <= 0 || trainPct >= 1 {
		return WalkForwardAnalysisReport{}, fmt.Errorf("invalid walk-forward parameters")
	}
	if embargo < 1 {
		embargo = 1
	}
	chunk := len(daily) / splitCount
	if chunk < 80 {
		return WalkForwardAnalysisReport{}, fmt.Errorf("not enough candles per split")
	}
	out := WalkForwardAnalysisReport{Enabled: true}
	for i := 0; i < splitCount; i++ {
		start := i * chunk
		end := start + chunk
		if i == splitCount-1 {
			end = len(daily)
		}
		trainEnd := start + int(float64(end-start)*trainPct)
		evalStart := trainEnd + embargo
		if evalStart >= end {
			continue
		}
		train := coreSignalStats(cfg, btc, max(start, 60), trainEnd)
		eval := coreSignalStats(cfg, btc, evalStart, end)
		out.Splits = append(out.Splits, WalkForwardAnalysisSplit{i, start, trainEnd, evalStart, end, embargo, train, eval})
	}
	if len(out.Splits) == 0 {
		return WalkForwardAnalysisReport{}, fmt.Errorf("no valid walk-forward analysis splits")
	}
	out.Summary = fmt.Sprintf("core signal walk-forward splits=%d embargo=%d research-only", len(out.Splits), embargo)
	return out, nil
}
func coreSignalStats(cfg config.Config, btc map[string][]market.Candle, start, end int) CoreSignalStats {
	s := CoreSignalStats{Permissions: map[agent1.Permission]int{}, Biases: map[flow.Bias]int{}}
	fg := exchange.FearGreed{Value: 50, Classification: "Neutral"}
	for i := start; i < end; i++ {
		window := btcTimeframeWindow(btc, i)
		a, err := agent1.Analyze(cfg, window, fg)
		if err != nil {
			continue
		}
		s.Samples++
		s.Permissions[a.ActionPermission]++
		s.Biases[a.Flow.Bias]++
	}
	return s
}
