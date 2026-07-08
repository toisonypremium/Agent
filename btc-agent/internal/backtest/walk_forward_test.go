package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestWalkForwardSimulationSplitsHistory(t *testing.T) {
	btc := map[string][]market.Candle{"1d": walkForwardCandles("BTCUSDT", 300)}
	assets := map[string][]market.Candle{"ETHUSDT": walkForwardCandles("ETHUSDT", 300)}
	cfg := config.Config{}
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Execution.OrderExpiryHours = 24

	report, err := RunWalkForwardSimulation(cfg, btc, assets, 2, 0.6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Enabled {
		t.Fatal("expected enabled report")
	}
	if len(report.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(report.Splits))
	}
	if report.Splits[0].TrainDays != 90 || report.Splits[0].EvalDays != 60 {
		t.Fatalf("unexpected split sizes: %+v", report.Splits[0])
	}
}

func TestWalkForwardSimulationRejectsTinyWindows(t *testing.T) {
	_, err := RunWalkForwardSimulation(config.Config{}, map[string][]market.Candle{"1d": walkForwardCandles("BTCUSDT", 80)}, nil, 2, 0.6)
	if err == nil {
		t.Fatal("expected not enough data error")
	}
}

func walkForwardCandles(symbol string, n int) []market.Candle {
	start := time.Unix(1700000000, 0)
	out := make([]market.Candle, n)
	for i := range out {
		open := start.AddDate(0, 0, i)
		price := 100 + float64(i%30)
		out[i] = market.Candle{
			Symbol:    symbol,
			Interval:  "1d",
			OpenTime:  open,
			CloseTime: open.Add(24 * time.Hour),
			Open:      price,
			High:      price + 2,
			Low:       price - 2,
			Close:     price,
			Volume:    1000,
		}
	}
	return out
}
