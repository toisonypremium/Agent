package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func triggerAuditConfig() config.Config {
	var cfg config.Config
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": .35, "SOLUSDT": .45, "RENDERUSDT": .20}
	cfg.Risk.MaxTotalDeploymentPerCycle = .70
	cfg.Risk.MaxSingleAssetDeployment = .45
	cfg.Risk.MinRewardRisk = 3
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	cfg.Execution.LayerDistribution = []float64{.25, .35, .40}
	cfg.Execution.OrderExpiryHours = 48
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	return cfg
}

func auditCandles(symbol string, n int, start float64) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Unix(1700000000, 0)
	for i := range out {
		price := start + float64((i*3)%30) + float64(i)*0.05
		out[i] = market.Candle{Symbol: symbol, Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: price * 1.001, High: price * 1.02, Low: price * 0.98, Close: price, Volume: 1000}
	}
	return out
}

func TestRunWatchlistTriggerAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": auditCandles("ETHUSDT", 140, 80), "SOLUSDT": auditCandles("SOLUSDT", 140, 60)}
	got, err := RunWatchlistTriggerAudit(cfg, btc, assets, WatchlistTriggerAuditConfig{ReadinessThresholds: []float64{0.30}, HorizonDays: []int{3, 7}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" {
		t.Fatalf("expected enabled result with summary: %+v", got)
	}
}

func TestWatchlistTriggerClassifiesFlowMissing(t *testing.T) {
	got := watchlistTrigger(agent2.WatchCandidate{Missing: []string{"asset flow chưa reclaim/absorption"}})
	if got != TriggerFlowNotConfirmed {
		t.Fatalf("trigger=%s want %s", got, TriggerFlowNotConfirmed)
	}
}

func TestWatchlistTriggerAuditVerdictRejectsSmallSample(t *testing.T) {
	row := WatchlistTriggerAuditRow{Count: 4, AvgReturn: map[int]float64{7: 0.10}, WinRate: map[int]float64{7: 1}, WorstDrawdown: map[int]float64{7: -0.01}}
	if got := watchlistTriggerAuditVerdict(row, []int{7}); got != "REJECT" {
		t.Fatalf("verdict=%s want REJECT", got)
	}
}

func TestWatchlistTriggerAuditDefaultsSkipUnactionable(t *testing.T) {
	cfg := triggerAuditConfig()
	got := normalizeWatchlistTriggerAuditConfig(cfg, WatchlistTriggerAuditConfig{})
	if got.IncludeUnactionable {
		t.Fatal("default should skip unactionable candidates")
	}
	want := []float64{0.60, 0.70, 0.80}
	if len(got.ReadinessThresholds) != len(want) {
		t.Fatalf("thresholds=%v", got.ReadinessThresholds)
	}
	for i := range want {
		if got.ReadinessThresholds[i] != want[i] {
			t.Fatalf("thresholds=%v want %v", got.ReadinessThresholds, want)
		}
	}
}
