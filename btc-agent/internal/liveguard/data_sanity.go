package liveguard

import (
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

const (
	DataSanityOK    = "DATA_SANITY_OK"
	DataSanityWarn  = "DATA_SANITY_WARN"
	DataSanityBlock = "DATA_SANITY_BLOCK"
)

const (
	maxBTC1DCandleAge = 48 * time.Hour
	maxBTC4HCandleAge = 12 * time.Hour
	maxBTC1WCandleAge = 10 * 24 * time.Hour
	wideZonePct       = 0.20
	farZonePct        = 0.35
)

type DataSanityResult struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Status      string                     `json:"status"`
	BTC         map[string]CandleFreshness `json:"btc"`
	Assets      []DataSanitySymbol         `json:"assets,omitempty"`
	Zones       []ZoneSanity               `json:"zones,omitempty"`
	Blockers    []string                   `json:"blockers,omitempty"`
	Warnings    []string                   `json:"warnings,omitempty"`
	Summary     string                     `json:"summary"`
}

type DataSanitySymbol struct {
	Symbol      string        `json:"symbol"`
	Interval    string        `json:"interval"`
	Count       int           `json:"count"`
	LatestClose float64       `json:"latest_close,omitempty"`
	LatestAt    time.Time     `json:"latest_at,omitempty"`
	Age         time.Duration `json:"age,omitempty"`
	Pass        bool          `json:"pass"`
	Reason      string        `json:"reason,omitempty"`
}

type CandleFreshness struct {
	Interval string        `json:"interval"`
	Count    int           `json:"count"`
	LatestAt time.Time     `json:"latest_at,omitempty"`
	Age      time.Duration `json:"age,omitempty"`
	MaxAge   time.Duration `json:"max_age"`
	Pass     bool          `json:"pass"`
	Reason   string        `json:"reason,omitempty"`
}

type ZoneSanity struct {
	Name            string      `json:"name"`
	Zone            market.Zone `json:"zone"`
	Valid           bool        `json:"valid"`
	WidthPct        float64     `json:"width_pct,omitempty"`
	DistanceFromBTC float64     `json:"distance_from_btc_pct,omitempty"`
	Pass            bool        `json:"pass"`
	Warning         bool        `json:"warning,omitempty"`
	Reason          string      `json:"reason,omitempty"`
}

func CheckDataSanity(cfg config.Config, btc map[string][]market.Candle, assets map[string][]market.Candle, analysis agent1.MarketAnalysis, now time.Time) DataSanityResult {
	if now.IsZero() {
		now = time.Now()
	}
	res := DataSanityResult{GeneratedAt: now, Status: DataSanityOK, BTC: map[string]CandleFreshness{}}
	for _, interval := range []string{"1d", "4h", "1w"} {
		maxAge := maxBTC1DCandleAge
		switch interval {
		case "4h":
			maxAge = maxBTC4HCandleAge
		case "1w":
			maxAge = maxBTC1WCandleAge
		}
		fresh := candleFreshness(interval, btc[interval], minCandlesForInterval(interval), maxAge, now)
		res.BTC[interval] = fresh
		if !fresh.Pass {
			res.Blockers = append(res.Blockers, fresh.Reason)
		}
	}
	for _, symbol := range cfg.Data.Symbols.Assets {
		s := dataSanitySymbol(symbol, "1d", assets[symbol], min1DCandles, max1DCandleAge, now)
		res.Assets = append(res.Assets, s)
		if !s.Pass {
			res.Blockers = append(res.Blockers, s.Reason)
		}
	}
	price := analysis.BTCPrice
	if price <= 0 {
		price = market.LastClose(btc["1d"])
	}
	zones := []struct {
		name string
		zone market.Zone
	}{
		{"support", analysis.PrimarySupportZone},
		{"deep_support", analysis.DeepSupportZone},
		{"resistance", analysis.ResistanceZone},
		{"accumulation", analysis.AccumulationZone},
		{"invalidation", analysis.InvalidationZone},
	}
	for _, item := range zones {
		z := checkZoneSanity(item.name, item.zone, price)
		res.Zones = append(res.Zones, z)
		if !z.Pass {
			res.Blockers = append(res.Blockers, z.Reason)
		} else if z.Warning {
			res.Warnings = append(res.Warnings, z.Reason)
		}
	}
	res.refreshDataSanitySummary()
	return res
}

func candleFreshness(interval string, candles []market.Candle, minCount int, maxAge time.Duration, now time.Time) CandleFreshness {
	f := CandleFreshness{Interval: interval, Count: len(candles), MaxAge: maxAge}
	if len(candles) < minCount {
		f.Reason = fmt.Sprintf("BTC %s candles=%d need>=%d", interval, len(candles), minCount)
		return f
	}
	latest := candles[len(candles)-1]
	if !validOHLC(latest) {
		f.Reason = fmt.Sprintf("BTC %s latest OHLC invalid", interval)
		return f
	}
	f.LatestAt = dataSanityCandleTime(latest, now)
	if f.LatestAt.IsZero() {
		f.Reason = fmt.Sprintf("BTC %s latest timestamp missing", interval)
		return f
	}
	f.Age = now.Sub(f.LatestAt)
	if f.Age < 0 || f.Age > maxAge {
		f.Reason = fmt.Sprintf("BTC %s stale: age=%s max=%s", interval, f.Age.Round(time.Minute), maxAge)
		return f
	}
	f.Pass = true
	f.Reason = fmt.Sprintf("BTC %s fresh age=%s candles=%d", interval, f.Age.Round(time.Minute), len(candles))
	return f
}

func dataSanitySymbol(symbol, interval string, candles []market.Candle, minCount int, maxAge time.Duration, now time.Time) DataSanitySymbol {
	s := DataSanitySymbol{Symbol: symbol, Interval: interval, Count: len(candles)}
	if len(candles) < minCount {
		s.Reason = fmt.Sprintf("%s %s candles=%d need>=%d", symbol, interval, len(candles), minCount)
		return s
	}
	latest := candles[len(candles)-1]
	s.LatestClose = latest.Close
	if !validOHLC(latest) {
		s.Reason = fmt.Sprintf("%s %s latest OHLC invalid", symbol, interval)
		return s
	}
	s.LatestAt = dataSanityCandleTime(latest, now)
	if s.LatestAt.IsZero() {
		s.Reason = fmt.Sprintf("%s %s latest timestamp missing", symbol, interval)
		return s
	}
	s.Age = now.Sub(s.LatestAt)
	if s.Age < 0 || s.Age > maxAge {
		s.Reason = fmt.Sprintf("%s %s stale: age=%s max=%s", symbol, interval, s.Age.Round(time.Minute), maxAge)
		return s
	}
	s.Pass = true
	s.Reason = fmt.Sprintf("%s %s fresh age=%s candles=%d", symbol, interval, s.Age.Round(time.Minute), len(candles))
	return s
}

func checkZoneSanity(name string, zone market.Zone, price float64) ZoneSanity {
	z := ZoneSanity{Name: name, Zone: zone, Valid: zone.Valid(), Pass: true}
	if !zone.Valid() {
		z.Pass = false
		z.Reason = name + " zone invalid"
		return z
	}
	mid := zone.Mid()
	if mid > 0 {
		z.WidthPct = (zone.High - zone.Low) / mid
	}
	if price > 0 {
		if price >= zone.Low && price <= zone.High {
			z.DistanceFromBTC = 0
		} else if price < zone.Low {
			z.DistanceFromBTC = (zone.Low - price) / price
		} else {
			z.DistanceFromBTC = (price - zone.High) / price
		}
	}
	warnings := []string{}
	if z.WidthPct > wideZonePct {
		warnings = append(warnings, fmt.Sprintf("%s zone rộng %.1f%%", name, z.WidthPct*100))
	}
	if math.Abs(z.DistanceFromBTC) > farZonePct {
		warnings = append(warnings, fmt.Sprintf("%s zone xa BTC %.1f%%", name, z.DistanceFromBTC*100))
	}
	if len(warnings) > 0 {
		z.Warning = true
		z.Reason = strings.Join(warnings, "; ")
	} else {
		z.Reason = fmt.Sprintf("%s zone OK width=%.1f%% distance=%.1f%%", name, z.WidthPct*100, z.DistanceFromBTC*100)
	}
	return z
}

func (r *DataSanityResult) refreshDataSanitySummary() {
	r.Blockers = uniqueHealthStrings(r.Blockers)
	r.Warnings = uniqueHealthStrings(r.Warnings)
	switch {
	case len(r.Blockers) > 0:
		r.Status = DataSanityBlock
	case len(r.Warnings) > 0:
		r.Status = DataSanityWarn
	default:
		r.Status = DataSanityOK
	}
	r.Summary = fmt.Sprintf("%s: btc_frames=%d assets=%d blockers=%d warnings=%d", r.Status, len(r.BTC), len(r.Assets), len(r.Blockers), len(r.Warnings))
}

func validOHLC(c market.Candle) bool {
	return c.Close > 0 && c.High > 0 && c.Low > 0 && c.Open > 0 && c.High >= c.Low && c.High >= c.Close && c.High >= c.Open && c.Low <= c.Close && c.Low <= c.Open
}

func minCandlesForInterval(interval string) int {
	switch interval {
	case "4h":
		return 80
	case "1w":
		return 52
	default:
		return 60
	}
}

func dataSanityCandleTime(c market.Candle, now time.Time) time.Time {
	if !c.CloseTime.IsZero() && !c.CloseTime.After(now) {
		return c.CloseTime
	}
	if !c.OpenTime.IsZero() && !c.OpenTime.After(now) {
		return c.OpenTime
	}
	if !c.CloseTime.IsZero() && c.CloseTime.After(now) && c.CloseTime.Before(now.Add(2*time.Hour)) {
		return now
	}
	return time.Time{}
}
