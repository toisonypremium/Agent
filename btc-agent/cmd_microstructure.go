package main

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/microstructure"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"strings"
	"time"
)

func runMicrostructureFetch(ctx context.Context, cfg config.Config, db *storage.DB) (microstructure.Summary, error) {
	if !cfg.Microstructure.Enabled {
		summary := microstructure.BuildSummary(false, cfg.Data.Symbols.BTC, nil, 0, time.Now().UTC())
		fmt.Println(microstructure.Markdown(summary))
		return summary, nil
	}
	summary, err := fetchMicrostructureSummary(ctx, cfg, db)
	if err != nil {
		return summary, err
	}
	if err := writeMicrostructureReport(summary); err != nil {
		return summary, err
	}
	fmt.Println(microstructure.Markdown(summary))
	return summary, nil
}

func fetchMicrostructureSummary(ctx context.Context, cfg config.Config, db *storage.DB) (microstructure.Summary, error) {
	spotBase := firstNonEmpty(cfg.Microstructure.BinanceSpotBaseURL, cfg.Data.BinanceBaseURL, "https://api.binance.com")
	futuresBase := firstNonEmpty(cfg.Microstructure.BinanceFuturesBaseURL, "https://fapi.binance.com")
	spot := exchange.NewBinance(spotBase)
	futures := exchange.NewBinanceFutures(futuresBase)
	now := time.Now().UTC()
	maxAge := time.Duration(microstructureMaxAgeMinutes(cfg)) * time.Minute
	snapshots := []microstructure.Snapshot{}
	for _, symbol := range microstructureSymbols(cfg) {
		snapshot := microstructure.Snapshot{Symbol: strings.ToUpper(symbol), Timestamp: now, Source: "binance-public"}
		if flow, latest, err := spot.KlineFlow(ctx, symbol, microstructureInterval(cfg), microstructureLookbackLimit(cfg)); err != nil {
			snapshot.Health.Blockers = append(snapshot.Health.Blockers, fmt.Sprintf("%s kline flow fetch: %v", strings.ToUpper(symbol), err))
		} else {
			snapshot.SpotFlow = flow
			if !latest.IsZero() {
				snapshot.Timestamp = latest.UTC()
			}
		}
		if book, err := spot.Depth(ctx, symbol, microstructureDepthLimit(cfg)); err != nil {
			snapshot.Health.Warnings = append(snapshot.Health.Warnings, fmt.Sprintf("%s orderbook fetch: %v", strings.ToUpper(symbol), err))
		} else {
			snapshot.OrderBook = book
		}
		if fut, err := futures.FuturesObservation(ctx, symbol); err != nil {
			snapshot.Health.Warnings = append(snapshot.Health.Warnings, fmt.Sprintf("%s futures observation fetch: %v", strings.ToUpper(symbol), err))
		} else {
			snapshot.Futures = fut
		}
		snapshot = microstructure.BuildSignals(snapshot)
		snapshot = microstructure.EvaluateHealth(snapshot, now, maxAge)
		snapshots = append(snapshots, snapshot)
	}
	if db != nil {
		if err := db.SaveMicrostructureSnapshots(snapshots); err != nil {
			return microstructure.Summary{}, fmt.Errorf("save microstructure snapshots: %w", err)
		}
	}
	summary := microstructure.BuildSummary(true, cfg.Data.Symbols.BTC, snapshots, microstructureRequiredFresh(cfg), now)
	// MM Footprint: load history từ DB, phân tích dấu vết MM qua nhiều snapshot
	if db != nil {
		history, err := db.LoadMicrostructureHistory(microstructureSymbols(cfg), 20)
		if err == nil && len(history) > 0 {
			summary.MMFootprint = microstructure.AnalyzeMMFootprintMulti(history)
		}
	}
	return summary, nil
}

func writeMicrostructureReport(summary microstructure.Summary) error {
	if err := saveJSONFile("reports", "microstructure_latest.json", microstructure.Report{GeneratedAt: time.Now().UTC(), Summary: summary}); err != nil {
		return err
	}
	return reportio.WriteMarkdown("reports", "microstructure_latest.md", microstructure.Markdown(summary))
}

func latestMicrostructureSummary(cfg config.Config, db *storage.DB, now time.Time) microstructure.Summary {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !cfg.Microstructure.Enabled || db == nil {
		return microstructure.BuildSummary(false, cfg.Data.Symbols.BTC, nil, 0, now)
	}
	snapshots, err := db.LatestMicrostructureSnapshots(microstructureSymbols(cfg))
	if err != nil {
		return microstructure.Summary{GeneratedAt: now, Enabled: true, Status: microstructure.StatusBlock, Blockers: []string{"load microstructure snapshots: " + err.Error()}, Summary: "MICROSTRUCTURE_BLOCK: load failed"}
	}
	return microstructure.BuildSummary(true, cfg.Data.Symbols.BTC, snapshots, microstructureRequiredFresh(cfg), now)
}

func applyMicrostructurePermissionGate(cfg config.Config, analysis agent1.MarketAnalysis, summary microstructure.Summary) agent1.MarketAnalysis {
	analysis.Microstructure = summary
	if !cfg.Microstructure.Enabled || !cfg.Microstructure.RequireFreshForActive || !microstructure.BlocksActive(summary) {
		return analysis
	}
	if analysis.ActionPermission == agent1.Allowed || analysis.ActionPermission == agent1.Armed {
		analysis.ActionPermission = agent1.Watch
	}
	message := "microstructure stale/missing; max WATCH"
	if analysis.PermissionReason != "" {
		analysis.PermissionReason += "; " + message
	} else {
		analysis.PermissionReason = message
	}
	analysis.Summary = fmt.Sprintf("BTC %.2f, regime %s, trend %.1f, permission %s", analysis.BTCPrice, analysis.MarketRegime, analysis.TrendScore, analysis.ActionPermission)
	return analysis
}

func applyMicrostructureAssetGate(cfg config.Config, p agent2.Plan, summary microstructure.Summary) agent2.Plan {
	if !cfg.Microstructure.Enabled || !cfg.Microstructure.RequireFreshForActive || !summary.Enabled {
		return p
	}
	changed := false
	for i := range p.Assets {
		asset := &p.Assets[i]
		if microstructure.SymbolFresh(summary, asset.Symbol) {
			continue
		}
		reason := strings.ToUpper(asset.Symbol) + " microstructure stale/missing; max WATCH"
		asset.Reasons = agent2.AddReason(asset.Reasons, agent2.NewDecisionReason(agent2.ReasonDataWait, agent2.ReasonSoftWait, agent2.ReasonScopeData, reason))
		asset.SoftBlockers = appendUniqueMain(asset.SoftBlockers, reason)
		asset.Layers = nil
		if asset.State == agent2.StateActiveLimit || asset.State == agent2.StateArmed || asset.State == agent2.StateScout {
			asset.State = agent2.StateWatch
			asset.Reason = reason
			asset.NextTrigger = "Chờ microstructure fresh trước khi xét ACTIVE_LIMIT."
			changed = true
		}
	}
	if changed && p.State == agent2.StateActiveLimit {
		p.State = agent2.StateWatch
		for _, asset := range p.Assets {
			if asset.State == agent2.StateActiveLimit {
				p.State = agent2.StateActiveLimit
				break
			}
			if asset.State == agent2.StateArmed && p.State != agent2.StateActiveLimit {
				p.State = agent2.StateArmed
			}
			if asset.State == agent2.StateScout && p.State != agent2.StateActiveLimit && p.State != agent2.StateArmed {
				p.State = agent2.StateScout
			}
		}
		if p.State != agent2.StateActiveLimit {
			p.Summary = "Microstructure stale/missing; không tạo ACTIVE_LIMIT."
		}
	}
	return p
}

func microstructureSymbols(cfg config.Config) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(symbol string) {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || seen[symbol] {
			return
		}
		seen[symbol] = true
		out = append(out, symbol)
	}
	add(cfg.Data.Symbols.BTC)
	for _, symbol := range cfg.Data.Symbols.Assets {
		add(symbol)
	}
	return out
}

func microstructureInterval(cfg config.Config) string {
	if cfg.Microstructure.Interval != "" {
		return cfg.Microstructure.Interval
	}
	return "5m"
}

func microstructureLookbackLimit(cfg config.Config) int {
	if cfg.Microstructure.LookbackLimit > 0 {
		return cfg.Microstructure.LookbackLimit
	}
	return 120
}

func microstructureMaxAgeMinutes(cfg config.Config) int {
	if cfg.Microstructure.MaxAgeMinutes > 0 {
		return cfg.Microstructure.MaxAgeMinutes
	}
	return 30
}

func microstructureDepthLimit(cfg config.Config) int {
	if cfg.Microstructure.OrderBookDepthLimit > 0 {
		return cfg.Microstructure.OrderBookDepthLimit
	}
	return 100
}

func microstructureRequiredFresh(cfg config.Config) int {
	if cfg.Microstructure.MinFreshSymbolsRequired > 0 {
		return cfg.Microstructure.MinFreshSymbolsRequired
	}
	return len(microstructureSymbols(cfg))
}

func appendUniqueMain(items []string, values ...string) []string {
	for _, value := range values {
		if value == "" {
			continue
		}
		found := false
		for _, item := range items {
			if item == value {
				found = true
				break
			}
		}
		if !found {
			items = append(items, value)
		}
	}
	return items
}
