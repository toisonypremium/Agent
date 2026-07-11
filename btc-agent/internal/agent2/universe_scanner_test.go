package agent2

import (
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestResearchUniverseSymbolsFallback(t *testing.T) {
	var cfg config.Config
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	got := ResearchUniverseSymbols(cfg)
	if len(got) != 2 || got[0] != "ETHUSDT" || got[1] != "SOLUSDT" {
		t.Fatalf("unexpected symbols: %v", got)
	}
}

func TestResearchUniverseSymbolsUsesResearchUniverse(t *testing.T) {
	var cfg config.Config
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Data.Symbols.ResearchUniverse = []string{"linkusdt", "BTCUSDT", "LINKUSDT", "avaxusdt"}
	got := ResearchUniverseSymbols(cfg)
	if len(got) != 2 || got[0] != "LINKUSDT" || got[1] != "AVAXUSDT" {
		t.Fatalf("unexpected symbols: %v", got)
	}
}

func TestBuildUniverseResearchReportMissingData(t *testing.T) {
	var cfg config.Config
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	cfg.Data.Symbols.ResearchUniverse = []string{"ETHUSDT", "LINKUSDT"}
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.3, "LINKUSDT": 0.1}
	cfg.Risk.MinRewardRisk = 2
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Watch}
	assets := map[string][]market.Candle{"ETHUSDT": universeCandles("ETHUSDT")}
	report := BuildUniverseResearchReport(cfg, analysis, assets, universeCandles("BTCUSDT"), time.Unix(1700000000, 0))
	if len(report.Rows) != 2 {
		t.Fatalf("rows=%d", len(report.Rows))
	}
	bySymbol := map[string]UniverseResearchRow{}
	for _, row := range report.Rows {
		bySymbol[row.Symbol] = row
	}
	if bySymbol["LINKUSDT"].DataStatus != UniverseDataMissing {
		t.Fatalf("expected LINK data missing: %+v", bySymbol["LINKUSDT"])
	}
	if len(report.TopCandidates) == 0 {
		t.Fatal("expected top candidates")
	}
}

func universeCandles(symbol string) []market.Candle {
	out := make([]market.Candle, 80)
	start := time.Unix(1700000000, 0).Add(-80 * 24 * time.Hour)
	for i := range out {
		open := start.Add(time.Duration(i) * 24 * time.Hour)
		price := 100 + float64(i)*0.2
		out[i] = market.Candle{Symbol: symbol, Interval: "1d", OpenTime: open, CloseTime: open.Add(24 * time.Hour), Open: price, High: price * 1.03, Low: price * 0.97, Close: price, Volume: 1000 + float64(i)}
	}
	return out
}
