package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/accumulation"
	"btc-agent/internal/market"
)

func phaseAuditCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := 100.0 + float64((i*3)%20)
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price, High: price + 2, Low: price - 2, Close: price + 0.5, Volume: 1000}
	}
	return out
}

func TestRunAccumulationPhaseAuditProducesRows(t *testing.T) {
	candles := phaseAuditCandles(140)
	for i := 70; i < len(candles); i += 15 {
		candles[i].Open = 97
		candles[i].High = 108
		candles[i].Low = 88
		candles[i].Close = 104
		candles[i].Volume = 2600
	}
	got := RunAccumulationPhaseAudit("BTCUSDT", candles, []int{1, 3, 7, 14})
	if !got.Enabled || len(got.Rows) == 0 {
		t.Fatalf("expected enabled rows: %+v", got)
	}
	seen := map[accumulation.Phase]bool{}
	for _, row := range got.Rows {
		seen[row.Phase] = true
		if row.Count <= 0 || row.AvgForwardReturn == nil || row.WorstMAE == nil || row.BestMFE == nil {
			t.Fatalf("bad row: %+v", row)
		}
	}
	if !seen[accumulation.PhaseSweep] && !seen[accumulation.PhaseReclaim] && !seen[accumulation.PhaseConfirmed] {
		t.Fatalf("expected accumulation-like phase: %+v", got.Rows)
	}
}

func TestRunAccumulationPhaseAuditLowDataSkips(t *testing.T) {
	got := RunAccumulationPhaseAudit("BTCUSDT", phaseAuditCandles(20), []int{1, 3, 7})
	if got.Enabled {
		t.Fatalf("expected skipped audit: %+v", got)
	}
}
