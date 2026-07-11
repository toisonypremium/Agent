package liveguard

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/market"
)

func TestCheckDataHealthBlocksStalePlanAndBadOrder(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := healthConfig()
	analysis := agent1.MarketAnalysis{Timestamp: now.Add(-time.Hour), ActionPermission: agent1.Allowed}
	plan := agent2.Plan{Timestamp: now.Add(-8 * time.Hour), State: agent2.StateWatch}
	assets := map[string][]market.Candle{"ETHUSDT": healthCandles(now)}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Status: live.StatusLiveOpen, Price: 100}}

	got := CheckDataHealth(cfg, analysis, plan, assets, open, nil, now)
	if got.Status != DataHealthBlock {
		t.Fatalf("status=%s summary=%s blockers=%v", got.Status, got.Summary, got.Blockers)
	}
	joined := strings.Join(got.Blockers, " ")
	if !strings.Contains(joined, "plan stale") || !strings.Contains(joined, "invalid live open order") {
		t.Fatalf("missing blockers: %+v", got)
	}
}

func TestCheckDataHealthOKCleanNoPositionState(t *testing.T) {
	// No open orders, no positions = normal state; expect DATA_HEALTH_OK not WARN.
	now := time.Unix(1700000000, 0)
	cfg := healthConfig()
	analysis := agent1.MarketAnalysis{Timestamp: now.Add(-time.Hour), ActionPermission: agent1.Allowed}
	plan := agent2.Plan{Timestamp: now.Add(-time.Hour), State: agent2.StateWatch}
	assets := map[string][]market.Candle{"ETHUSDT": healthCandles(now)}

	got := CheckDataHealth(cfg, analysis, plan, assets, nil, nil, now)
	if got.Status != DataHealthOK {
		t.Fatalf("expected DATA_HEALTH_OK for clean state, got status=%s summary=%s blockers=%v warnings=%v", got.Status, got.Summary, got.Blockers, got.Warnings)
	}
	if len(got.Blockers) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("expected no blockers/warnings for clean state: %+v", got)
	}
}

func TestCheckDataHealthOKValidOpenOrder(t *testing.T) {
	// Valid open order = no WARN for it; only validate fields.
	now := time.Unix(1700000000, 0)
	cfg := healthConfig()
	analysis := agent1.MarketAnalysis{Timestamp: now.Add(-time.Hour), ActionPermission: agent1.Allowed}
	plan := agent2.Plan{Timestamp: now.Add(-time.Hour), State: agent2.StateWatch}
	assets := map[string][]market.Candle{"ETHUSDT": healthCandles(now)}
	open := []live.OrderStatus{{InstID: "ETH-USDT", ClientOrderID: "c1", Status: live.StatusLiveOpen, Price: 100, Quantity: 0.01, Notional: 1}}

	got := CheckDataHealth(cfg, analysis, plan, assets, open, nil, now)
	if got.Status != DataHealthOK {
		t.Fatalf("expected DATA_HEALTH_OK for valid open order, got status=%s blockers=%v warnings=%v", got.Status, got.Blockers, got.Warnings)
	}
}

func TestCheckDataHealthUsesCurrentDailyOpenWhenCloseIsFuture(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := healthConfig()
	analysis := agent1.MarketAnalysis{Timestamp: now.Add(-time.Hour), ActionPermission: agent1.Allowed}
	plan := agent2.Plan{Timestamp: now.Add(-time.Hour), State: agent2.StateWatch}
	candles := healthCandles(now)
	candles[len(candles)-1].OpenTime = now.Add(-2 * time.Hour)
	candles[len(candles)-1].CloseTime = now.Add(22 * time.Hour)
	assets := map[string][]market.Candle{"ETHUSDT": candles}

	got := CheckDataHealth(cfg, analysis, plan, assets, nil, nil, now)
	if got.Status != DataHealthOK {
		t.Fatalf("expected DATA_HEALTH_OK using current daily open for in-progress candle, got status=%s blockers=%v warnings=%v", got.Status, got.Blockers, got.Warnings)
	}
}

func healthConfig() config.Config {
	var cfg config.Config
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	return cfg
}

func healthCandles(now time.Time) []market.Candle {
	out := make([]market.Candle, 60)
	start := now.Add(-60 * 24 * time.Hour)
	for i := range out {
		open := start.Add(time.Duration(i) * 24 * time.Hour)
		out[i] = market.Candle{Symbol: "ETHUSDT", Interval: "1d", OpenTime: open, CloseTime: open.Add(24 * time.Hour), Open: 100, High: 105, Low: 95, Close: 100, Volume: 1000}
	}
	out[len(out)-1].CloseTime = now.Add(-time.Hour)
	return out
}
