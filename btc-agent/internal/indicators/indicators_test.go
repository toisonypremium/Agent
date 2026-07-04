package indicators

import (
	"math"
	"testing"
)

func TestEMAIncreasing(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6}
	ema := EMA(xs, 3)
	if ema[len(ema)-1] <= ema[0] {
		t.Fatalf("EMA did not increase: %v", ema)
	}
}

func TestRSIRange(t *testing.T) {
	xs := []float64{1, 2, 3, 2, 4, 5, 4, 6, 7, 8, 7, 9, 10, 11, 10, 12, 13}
	rsi := RSI(xs, 14)
	got := rsi[len(rsi)-1]
	if got < 0 || got > 100 {
		t.Fatalf("RSI out of range: %v", got)
	}
}

func TestMACDValues(t *testing.T) {
	xs := make([]float64, 60)
	for i := range xs {
		xs[i] = float64(i) + 1
	}
	macd := MACD(xs, 12, 26, 9)
	if math.IsNaN(macd[len(macd)-1].MACD) {
		t.Fatal("MACD NaN")
	}
}

func TestATRPositive(t *testing.T) {
	highs := make([]float64, 20)
	lows := make([]float64, 20)
	closes := make([]float64, 20)
	for i := range closes {
		closes[i] = float64(10 + i)
		highs[i] = closes[i] + 1
		lows[i] = closes[i] - 1
	}
	atr := ATR(highs, lows, closes, 14)
	if atr[len(atr)-1] <= 0 {
		t.Fatalf("ATR not positive: %v", atr[len(atr)-1])
	}
}
