package market

import (
	"testing"
	"time"
)

func TestRangeZoneWidthValidInFlatMarket(t *testing.T) {
	candles := make([]Candle, 60)
	now := time.Unix(1700000000, 0)
	for i := range candles {
		candles[i] = Candle{OpenTime: now.Add(time.Duration(i) * time.Hour), Open: 100, High: 100, Low: 100, Close: 100, Volume: 1}
	}
	support, resistance := RangeZone(candles, 60)
	if !support.Valid() || !resistance.Valid() {
		t.Fatalf("zones invalid: support=%+v resistance=%+v", support, resistance)
	}
}
