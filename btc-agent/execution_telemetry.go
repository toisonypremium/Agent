package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/storage"
)

type executionTelemetryHealth struct {
	GeneratedAt    time.Time `json:"generated_at"`
	Status         string    `json:"status"`
	Symbols        []string  `json:"symbols"`
	CandlesSaved   int       `json:"candles_saved"`
	PendingWindows int       `json:"pending_windows"`
	StaleSymbols   []string  `json:"stale_symbols,omitempty"`
	Errors         []string  `json:"errors,omitempty"`
	Summary        string    `json:"summary"`
}

// collectExecutionTelemetry fetches only public 1m candles needed by active
// inventory/open orders and pending fill markouts. Failure is observability-only:
// callers record a warning and never grant or remove execution authority.
func collectExecutionTelemetry(ctx context.Context, cfg config.Config, db *storage.DB, now time.Time) (executionTelemetryHealth, error) {
	h := executionTelemetryHealth{GeneratedAt: now, Status: "TELEMETRY_OK", Symbols: []string{}, StaleSymbols: []string{}, Errors: []string{}}
	windows, err := db.PendingExecutionMarkoutWindows(now)
	if err != nil {
		return h, err
	}
	h.PendingWindows = len(windows)
	symbols := map[string]bool{}
	if ps, e := db.LivePositions(); e == nil {
		for _, x := range ps {
			if x.Quantity > 0 {
				symbols[strings.ToUpper(x.Symbol)] = true
			}
		}
	} else {
		h.Errors = append(h.Errors, "positions: "+e.Error())
	}
	if os, e := db.OpenLiveOrdersDetailed(); e == nil {
		for _, x := range os {
			symbols[strings.ToUpper(executionTelemetrySymbol(x.Symbol, live.InternalSymbol(x.InstID)))] = true
		}
	} else {
		h.Errors = append(h.Errors, "orders: "+e.Error())
	}
	for _, w := range windows {
		symbols[w.Symbol] = true
	}
	for x := range symbols {
		if x != "" {
			h.Symbols = append(h.Symbols, x)
		}
	}
	sort.Strings(h.Symbols)
	client := exchange.NewBinance(cfg.Data.BinanceBaseURL)
	for _, w := range windows {
		cs, e := client.KlinesRange(ctx, w.Symbol, "1m", 100, w.From, w.To)
		if e != nil {
			h.Errors = append(h.Errors, fmt.Sprintf("%s pending window: %v", w.Symbol, e))
			continue
		}
		if e = db.SaveCandles(cs); e != nil {
			h.Errors = append(h.Errors, fmt.Sprintf("%s save window: %v", w.Symbol, e))
			continue
		}
		h.CandlesSaved += len(cs)
	}
	for _, sym := range h.Symbols {
		cs, e := client.Klines(ctx, sym, "1m", 120)
		if e != nil {
			h.Errors = append(h.Errors, fmt.Sprintf("%s latest: %v", sym, e))
			continue
		}
		if e = db.SaveCandles(cs); e != nil {
			h.Errors = append(h.Errors, fmt.Sprintf("%s save latest: %v", sym, e))
			continue
		}
		h.CandlesSaved += len(cs)
		latest, ok, e := db.LatestCandleCloseTime(sym, "1m")
		if e != nil || !ok || now.Sub(latest) > 3*time.Minute {
			h.StaleSymbols = append(h.StaleSymbols, sym)
		}
	}
	if len(h.Errors) > 0 || len(h.StaleSymbols) > 0 {
		h.Status = "TELEMETRY_WARN"
	}
	h.Summary = fmt.Sprintf("%s: symbols=%d candles_saved=%d pending_windows=%d stale=%d errors=%d", h.Status, len(h.Symbols), h.CandlesSaved, h.PendingWindows, len(h.StaleSymbols), len(h.Errors))
	b, _ := json.Marshal(h)
	_, _ = db.Exec(`INSERT INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('execution_telemetry',?,?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at,payload_json=excluded.payload_json`, now.Unix(), string(b))
	return h, nil
}

func executionTelemetrySymbol(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
