package backtest

import (
	"fmt"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

// WalkForwardReport is a train/evaluate split report for Agent2 simulation.
type WalkForwardReport struct {
	Enabled bool               `json:"enabled"`
	Summary string             `json:"summary"`
	Splits  []WalkForwardSplit `json:"splits"`
}

type WalkForwardSplit struct {
	SplitIndex int              `json:"split_index"`
	TrainDays  int              `json:"train_days"`
	EvalDays   int              `json:"eval_days"`
	Train      Agent2Simulation `json:"train"`
	Eval       Agent2Simulation `json:"eval"`
}

// RunWalkForwardSimulation cuts local candle history into train/evaluate windows.
// It is research-only and does not tune config or change execution behavior.
func RunWalkForwardSimulation(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, splitCount int, trainPct float64) (WalkForwardReport, error) {
	btc1d := btc["1d"]
	if splitCount <= 0 {
		return WalkForwardReport{}, fmt.Errorf("split count must be positive")
	}
	if trainPct <= 0 || trainPct >= 1 {
		return WalkForwardReport{}, fmt.Errorf("train pct must be between 0 and 1")
	}
	if len(btc1d) < 100 {
		return WalkForwardReport{}, fmt.Errorf("not enough BTC 1d candles for walk-forward; need 100 got %d", len(btc1d))
	}

	total := len(btc1d)
	chunkSize := total / splitCount
	if chunkSize < 90 {
		return WalkForwardReport{}, fmt.Errorf("walk-forward split size %d too small", chunkSize)
	}

	report := WalkForwardReport{Enabled: true}
	for i := 0; i < splitCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if i == splitCount-1 {
			end = total
		}
		windowLen := end - start
		trainLen := int(float64(windowLen) * trainPct)
		if trainLen < 90 || windowLen-trainLen < 10 {
			continue
		}

		chunkBTC := candleMapSlice(btc, start, end)
		chunkAssets := candleMapSlice(assets, start, end)
		trainBTC := candleMapSlice(chunkBTC, 0, trainLen)
		trainAssets := candleMapSlice(chunkAssets, 0, trainLen)
		evalBTC := candleMapSlice(chunkBTC, trainLen, windowLen)
		evalAssets := candleMapSlice(chunkAssets, trainLen, windowLen)

		trainSim, trainErr := RunAgent2Simulation(cfg, trainBTC, trainAssets)
		if trainErr != nil {
			trainSim = Agent2Simulation{Enabled: false, Assets: map[string]AssetSimStats{}, Summary: trainErr.Error()}
		}
		evalSim, evalErr := RunAgent2Simulation(cfg, evalBTC, evalAssets)
		if evalErr != nil {
			evalSim = Agent2Simulation{Enabled: false, Assets: map[string]AssetSimStats{}, Summary: evalErr.Error()}
		}

		report.Splits = append(report.Splits, WalkForwardSplit{
			SplitIndex: i,
			TrainDays:  trainLen,
			EvalDays:   windowLen - trainLen,
			Train:      trainSim,
			Eval:       evalSim,
		})
	}
	if len(report.Splits) == 0 {
		return WalkForwardReport{}, fmt.Errorf("no valid walk-forward splits")
	}
	report.Summary = fmt.Sprintf("walk-forward split audit: splits=%d train_pct=%.2f research-only", len(report.Splits), trainPct)
	return report, nil
}

func candleMapSlice(in map[string][]market.Candle, start, end int) map[string][]market.Candle {
	out := map[string][]market.Candle{}
	for key, candles := range in {
		s := start
		e := end
		if s < 0 {
			s = 0
		}
		if e > len(candles) {
			e = len(candles)
		}
		if s >= e || s >= len(candles) {
			out[key] = nil
			continue
		}
		out[key] = candles[s:e]
	}
	return out
}
