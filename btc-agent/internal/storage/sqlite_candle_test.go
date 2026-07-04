package storage

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/market"
)

func TestLatestCandleOpenTime(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, found, err := db.LatestCandleOpenTime("BTCUSDT", "1d")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected no latest candle in empty DB")
	}

	oldest := time.Unix(1700000000, 0)
	newest := oldest.Add(24 * time.Hour)
	if err := db.SaveCandles([]market.Candle{
		{Symbol: "BTCUSDT", Interval: "1d", OpenTime: oldest, CloseTime: oldest.Add(24*time.Hour - time.Second), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 10},
		{Symbol: "BTCUSDT", Interval: "1d", OpenTime: newest, CloseTime: newest.Add(24*time.Hour - time.Second), Open: 2, High: 3, Low: 1.5, Close: 2.5, Volume: 11},
	}); err != nil {
		t.Fatal(err)
	}

	got, found, err := db.LatestCandleOpenTime("BTCUSDT", "1d")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected latest candle")
	}
	if !got.Equal(newest) {
		t.Fatalf("latest=%s want %s", got, newest)
	}
}
