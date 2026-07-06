package liveguard

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestCheckDataSanityOK(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	btc := map[string][]market.Candle{
		"1d": candlesForSanity("BTCUSDT", "1d", 80, now.Add(-24*time.Hour), 100),
		"4h": candlesForSanity("BTCUSDT", "4h", 100, now.Add(-4*time.Hour), 100),
		"1w": candlesForSanity("BTCUSDT", "1w", 60, now.Add(-7*24*time.Hour), 100),
	}
	assets := map[string][]market.Candle{"ETHUSDT": candlesForSanity("ETHUSDT", "1d", 80, now.Add(-24*time.Hour), 10)}
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	analysis := agent1.MarketAnalysis{BTCPrice: 100, PrimarySupportZone: market.Zone{Low: 90, High: 92}, DeepSupportZone: market.Zone{Low: 80, High: 84}, ResistanceZone: market.Zone{Low: 115, High: 118}, AccumulationZone: market.Zone{Low: 90, High: 92}, InvalidationZone: market.Zone{Low: 88, High: 90}}
	got := CheckDataSanity(cfg, btc, assets, analysis, now)
	if got.Status != DataSanityOK {
		t.Fatalf("status=%s blockers=%v warnings=%v", got.Status, got.Blockers, got.Warnings)
	}
}

func TestCheckDataSanityBlocksInvalidAndStale(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	btc := map[string][]market.Candle{
		"1d": candlesForSanity("BTCUSDT", "1d", 80, now.Add(-72*time.Hour), 100),
		"4h": candlesForSanity("BTCUSDT", "4h", 100, now.Add(-4*time.Hour), 100),
		"1w": candlesForSanity("BTCUSDT", "1w", 60, now.Add(-7*24*time.Hour), 100),
	}
	assets := map[string][]market.Candle{"ETHUSDT": candlesForSanity("ETHUSDT", "1d", 80, now.Add(-24*time.Hour), 10)}
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	analysis := agent1.MarketAnalysis{BTCPrice: 100, PrimarySupportZone: market.Zone{}, ResistanceZone: market.Zone{Low: 115, High: 118}, AccumulationZone: market.Zone{Low: 90, High: 92}, InvalidationZone: market.Zone{Low: 88, High: 90}}
	got := CheckDataSanity(cfg, btc, assets, analysis, now)
	if got.Status != DataSanityBlock {
		t.Fatalf("status=%s blockers=%v", got.Status, got.Blockers)
	}
	joined := strings.Join(got.Blockers, ";")
	if !strings.Contains(joined, "BTC 1d stale") || !strings.Contains(joined, "support zone invalid") {
		t.Fatalf("missing blockers: %v", got.Blockers)
	}
}

func TestCheckDataSanityWarnsWideZone(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	btc := map[string][]market.Candle{
		"1d": candlesForSanity("BTCUSDT", "1d", 80, now.Add(-24*time.Hour), 100),
		"4h": candlesForSanity("BTCUSDT", "4h", 100, now.Add(-4*time.Hour), 100),
		"1w": candlesForSanity("BTCUSDT", "1w", 60, now.Add(-7*24*time.Hour), 100),
	}
	assets := map[string][]market.Candle{"ETHUSDT": candlesForSanity("ETHUSDT", "1d", 80, now.Add(-24*time.Hour), 10)}
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	analysis := agent1.MarketAnalysis{BTCPrice: 100, PrimarySupportZone: market.Zone{Low: 50, High: 90}, DeepSupportZone: market.Zone{Low: 80, High: 84}, ResistanceZone: market.Zone{Low: 115, High: 118}, AccumulationZone: market.Zone{Low: 50, High: 90}, InvalidationZone: market.Zone{Low: 48, High: 50}}
	got := CheckDataSanity(cfg, btc, assets, analysis, now)
	if got.Status != DataSanityWarn {
		t.Fatalf("status=%s warnings=%v blockers=%v", got.Status, got.Warnings, got.Blockers)
	}
	if !strings.Contains(strings.Join(got.Warnings, ";"), "zone rộng") {
		t.Fatalf("expected wide zone warning: %v", got.Warnings)
	}
}

func candlesForSanity(symbol, interval string, n int, latest time.Time, close float64) []market.Candle {
	out := make([]market.Candle, n)
	step := 24 * time.Hour
	if interval == "4h" {
		step = 4 * time.Hour
	} else if interval == "1w" {
		step = 7 * 24 * time.Hour
	}
	start := latest.Add(-time.Duration(n-1) * step)
	for i := range out {
		at := start.Add(time.Duration(i) * step)
		px := close + float64(i%5)
		out[i] = market.Candle{Symbol: symbol, Interval: interval, OpenTime: at.Add(-step), CloseTime: at, Open: px, High: px * 1.02, Low: px * 0.98, Close: px, Volume: 1000}
	}
	return out
}
