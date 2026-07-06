package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func timeframeCandles(symbol, interval string, start time.Time, step time.Duration, n int, price float64) []market.Candle {
	out := make([]market.Candle, n)
	for i := range out {
		open := start.Add(time.Duration(i) * step)
		out[i] = market.Candle{Symbol: symbol, Interval: interval, OpenTime: open, CloseTime: open.Add(step), Open: price, High: price + 1, Low: price - 1, Close: price, Volume: 1000}
	}
	return out
}

func TestBTCTimeframeWindowUsesRealFramesUntilDailyClose(t *testing.T) {
	start := time.Unix(1700000000, 0)
	daily := timeframeCandles("BTCUSDT", "1d", start, 24*time.Hour, 5, 100)
	h4 := timeframeCandles("BTCUSDT", "4h", start, 4*time.Hour, 40, 100)
	weekly := timeframeCandles("BTCUSDT", "1w", start.Add(-14*24*time.Hour), 7*24*time.Hour, 4, 100)

	got := btcTimeframeWindow(map[string][]market.Candle{"1d": daily, "4h": h4, "1w": weekly}, 2)
	cutoff := daily[2].CloseTime
	if len(got["1d"]) != 3 {
		t.Fatalf("1d len=%d want 3", len(got["1d"]))
	}
	if len(got["4h"]) == 0 || got["4h"][len(got["4h"])-1].CloseTime.After(cutoff) {
		t.Fatalf("4h not clipped to cutoff %s: len=%d", cutoff, len(got["4h"]))
	}
	if len(got["1w"]) == 0 || got["1w"][len(got["1w"])-1].CloseTime.After(cutoff) {
		t.Fatalf("1w not clipped to cutoff %s: len=%d", cutoff, len(got["1w"]))
	}
	if len(got["4h"]) == len(got["1d"]) {
		t.Fatalf("4h should use real frame, got same len as daily: %d", len(got["4h"]))
	}
}

func TestBTCTimeframeWindowFallsBackToDailyWhenFramesMissing(t *testing.T) {
	daily := timeframeCandles("BTCUSDT", "1d", time.Unix(1700000000, 0), 24*time.Hour, 5, 100)
	got := btcTimeframeWindow(map[string][]market.Candle{"1d": daily}, 2)
	for _, tf := range []string{"1d", "4h", "1w"} {
		if len(got[tf]) != 3 {
			t.Fatalf("%s len=%d want daily fallback len 3", tf, len(got[tf]))
		}
	}
}

func TestBTCPermissionAuditUsesDivergentTimeframes(t *testing.T) {
	start := time.Unix(1700000000, 0)
	btc := map[string][]market.Candle{
		"1d": trendFrame("BTCUSDT", "1d", start, 24*time.Hour, 120, 100, 0.1),
		"4h": trendFrame("BTCUSDT", "4h", start, 4*time.Hour, 720, 100, 1.0),
		"1w": trendFrame("BTCUSDT", "1w", start.Add(-700*24*time.Hour), 7*24*time.Hour, 120, 200, -1.0),
	}
	got, err := RunBTCPermissionAudit(config.Config{}, btc, BTCPermissionAuditConfig{MinWindow1D: 60, HorizonDays: []int{3}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var row BTCPermissionScoreRow
	for _, r := range got.ScoreRows {
		if r.Count > 0 {
			row = r
			break
		}
	}
	if row.Count == 0 {
		t.Fatalf("expected score row with samples: %+v", got.ScoreRows)
	}
	if row.AvgWeeklyTrend == row.AvgDailyTrend && row.AvgDailyTrend == row.AvgFourHourTrend {
		t.Fatalf("expected divergent timeframe scores, got %+v", row)
	}
}

func trendFrame(symbol, interval string, start time.Time, step time.Duration, n int, first, delta float64) []market.Candle {
	out := make([]market.Candle, n)
	for i := range out {
		openTime := start.Add(time.Duration(i) * step)
		price := first + float64(i)*delta
		if price < 1 {
			price = 1
		}
		out[i] = market.Candle{Symbol: symbol, Interval: interval, OpenTime: openTime, CloseTime: openTime.Add(step), Open: price, High: price + 2, Low: price - 2, Close: price, Volume: 1000}
	}
	return out
}
