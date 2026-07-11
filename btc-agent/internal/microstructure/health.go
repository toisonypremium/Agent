package microstructure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func EvaluateHealth(s Snapshot, now time.Time, maxAge time.Duration) Snapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}
	s.Health.MaxAge = maxAge
	if s.Timestamp.IsZero() {
		s.Health.Blockers = append(s.Health.Blockers, "microstructure timestamp missing")
	} else {
		s.Health.Age = now.Sub(s.Timestamp)
		if s.Health.Age < 0 || s.Health.Age > maxAge {
			s.Health.Blockers = append(s.Health.Blockers, fmt.Sprintf("%s microstructure stale: age=%s max=%s", strings.ToUpper(s.Symbol), s.Health.Age.Round(time.Minute), maxAge))
		}
	}
	if s.SpotFlow.QuoteVolumeUSDT <= 0 {
		s.Health.Blockers = append(s.Health.Blockers, strings.ToUpper(s.Symbol)+" spot flow missing")
	}
	if s.OrderBook.BestBid <= 0 || s.OrderBook.BestAsk <= 0 {
		s.Health.Warnings = append(s.Health.Warnings, strings.ToUpper(s.Symbol)+" orderbook missing")
	}
	if s.Futures.OpenInterest <= 0 {
		s.Health.Warnings = append(s.Health.Warnings, strings.ToUpper(s.Symbol)+" futures OI missing")
	}
	s.Health.Blockers = unique(s.Health.Blockers)
	s.Health.Warnings = unique(s.Health.Warnings)
	s.Health.Fresh = len(s.Health.Blockers) == 0
	return s
}

func BuildSummary(enabled bool, btcSymbol string, snapshots []Snapshot, requiredFresh int, now time.Time) Summary {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := Summary{GeneratedAt: now, Enabled: enabled, Status: StatusOK, RequiredFresh: requiredFresh, Snapshots: snapshots, BySymbol: map[string]Snapshot{}}
	if !enabled {
		out.Status = StatusOK
		out.Summary = "microstructure disabled"
		out.Fingerprint = fingerprint(out)
		return out
	}
	for _, snapshot := range snapshots {
		symbol := strings.ToUpper(snapshot.Symbol)
		if symbol == "" {
			continue
		}
		out.BySymbol[symbol] = snapshot
		if snapshot.Health.Fresh {
			out.FreshSymbols++
		}
		out.Blockers = append(out.Blockers, snapshot.Health.Blockers...)
		out.Warnings = append(out.Warnings, snapshot.Health.Warnings...)
		if strings.EqualFold(symbol, btcSymbol) || (strings.EqualFold(symbol, "BTCUSDT") && out.BTC.Symbol == "") {
			out.BTC = snapshot
		}
	}
	if requiredFresh < 0 {
		requiredFresh = 0
	}
	if requiredFresh > 0 && out.FreshSymbols < requiredFresh {
		out.Blockers = append(out.Blockers, fmt.Sprintf("fresh microstructure symbols %d below required %d", out.FreshSymbols, requiredFresh))
	}
	if out.BTC.Symbol == "" {
		out.Blockers = append(out.Blockers, "BTC microstructure missing")
	} else if !out.BTC.Health.Fresh {
		out.Blockers = append(out.Blockers, "BTC microstructure stale/missing")
	}
	out.Blockers = unique(out.Blockers)
	out.Warnings = unique(out.Warnings)
	switch {
	case len(out.Blockers) > 0:
		out.Status = StatusBlock
	case len(out.Warnings) > 0:
		out.Status = StatusWarn
	default:
		out.Status = StatusOK
	}
	out.Summary = fmt.Sprintf("%s: fresh=%d/%d snapshots=%d blockers=%d warnings=%d", out.Status, out.FreshSymbols, maxInt(requiredFresh, len(snapshots)), len(snapshots), len(out.Blockers), len(out.Warnings))
	out.Fingerprint = fingerprint(out)
	return out
}

func BlocksActive(s Summary) bool {
	return s.Enabled && s.Status == StatusBlock
}

func SymbolFresh(s Summary, symbol string) bool {
	if !s.Enabled {
		return true
	}
	snapshot, ok := s.BySymbol[strings.ToUpper(symbol)]
	return ok && snapshot.Health.Fresh
}

func unique(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func fingerprint(s Summary) string {
	type row struct {
		Symbol       string `json:"symbol"`
		Fresh        bool   `json:"fresh"`
		BuyPressure  string `json:"buy_pressure"`
		CVDTrend     string `json:"cvd_trend"`
		OrderBook    string `json:"orderbook"`
		Funding      string `json:"funding"`
		Basis        string `json:"basis"`
		Blockers     int    `json:"blockers"`
		Warnings     int    `json:"warnings"`
		TakerBucket  int    `json:"taker_bucket"`
		CVDBucket    int    `json:"cvd_bucket"`
		SpreadBucket int    `json:"spread_bucket"`
	}
	rows := []row{}
	for _, snapshot := range s.Snapshots {
		rows = append(rows, row{Symbol: strings.ToUpper(snapshot.Symbol), Fresh: snapshot.Health.Fresh, BuyPressure: snapshot.Signals.BuyPressure, CVDTrend: snapshot.Signals.CVDTrend, OrderBook: snapshot.Signals.OrderBookBias, Funding: snapshot.Signals.FundingBias, Basis: snapshot.Signals.BasisBias, Blockers: len(snapshot.Health.Blockers), Warnings: len(snapshot.Health.Warnings), TakerBucket: bucket(snapshot.SpotFlow.TakerBuyRatio*100, 5), CVDBucket: bucket(snapshot.SpotFlow.CVDQuoteUSDT, 100000), SpreadBucket: bucket(snapshot.OrderBook.SpreadBps, 5)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Symbol < rows[j].Symbol })
	stable := struct {
		Enabled      bool   `json:"enabled"`
		Status       string `json:"status"`
		FreshSymbols int    `json:"fresh_symbols"`
		Required     int    `json:"required"`
		Blockers     int    `json:"blockers"`
		Warnings     int    `json:"warnings"`
		Rows         []row  `json:"rows"`
	}{s.Enabled, s.Status, s.FreshSymbols, s.RequiredFresh, len(s.Blockers), len(s.Warnings), rows}
	b, _ := json.Marshal(stable)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func bucket(value float64, step int) int {
	if step <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return int(math.Round(value/float64(step))) * step
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
