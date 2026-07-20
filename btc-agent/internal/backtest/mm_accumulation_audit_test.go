package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestMMAccumulationAuditIncludesForwardMAEMFE(t *testing.T) {
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	candles := make([]market.Candle, 130)
	for i := range candles {
		p := 100 + float64(i)/10
		candles[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: time.Unix(int64(i)*86400, 0), CloseTime: time.Unix(int64(i+1)*86400-1, 0), Open: p, High: p + 2, Low: p - 2, Close: p + 1, Volume: 1000}
	}
	got := RunMMAccumulationAudit(cfg, map[string][]market.Candle{"ETHUSDT": candles})
	if got.EvidenceSource != "OHLCV_ACCUMULATION_STRUCTURE" || len(got.Horizons) != 5 || len(got.Rows) != 1 {
		t.Fatalf("unexpected audit metadata: %+v", got)
	}
	if len(got.Rows[0].ByCase) == 0 {
		t.Fatalf("missing case forward stats: %+v", got.Rows[0])
	}
	for caseName, stats := range got.Rows[0].ByCase {
		if stats.Samples == 0 || len(stats.AvgReturn) != 5 || len(stats.AvgMAE) != 5 || len(stats.AvgMFE) != 5 {
			t.Fatalf("missing forward stats for %s: %+v", caseName, stats)
		}
	}
}

func TestMMForwardStatsMarksSmallSamplesInsufficient(t *testing.T) {
	a := newMMForwardAcc([]int{1})
	c := []market.Candle{{Close: 100}, {High: 102, Low: 98, Close: 101}}
	a.add(c, 0, []int{1})
	got := a.finalize([]int{1})
	if got.SampleQuality != "INSUFFICIENT_SAMPLE" || got.AvgMAE[1] >= 0 || got.AvgMFE[1] <= 0 {
		t.Fatalf("unexpected stats: %+v", got)
	}
}
