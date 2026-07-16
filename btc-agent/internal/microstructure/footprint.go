package microstructure

import (
	"math"
	"sort"
	"strings"
	"time"
)

// MMFootprintSignal combines time-normalized, volume-normalized market-maker
// footprint evidence. Funding and basis are context only; at least one executed
// flow/order-book core signal is required for any positive verdict.
type MMFootprintSignal struct {
	SnapshotCount      int     `json:"snapshot_count"`
	HistoryMinutes     float64 `json:"history_minutes"`
	CoveragePct        float64 `json:"coverage_pct"`
	DataQuality        float64 `json:"data_quality"`
	Fresh              bool    `json:"fresh"`
	CurrentAskPressure bool    `json:"current_ask_pressure"`

	CVDLatest          float64 `json:"cvd_latest"`
	CVDEarliest        float64 `json:"cvd_earliest"`
	CVDDelta           float64 `json:"cvd_delta"`
	CVDSlope           float64 `json:"cvd_slope"`
	NormalizedCVDSlope float64 `json:"normalized_cvd_slope"`
	PriceLatest        float64 `json:"price_latest"`
	PriceEarliest      float64 `json:"price_earliest"`
	PriceDeltaPct      float64 `json:"price_delta_pct"`
	CVDPriceDivergence bool    `json:"cvd_price_divergence"`

	TakerBuyBaseline float64 `json:"taker_buy_baseline"`
	TakerBuyMAD      float64 `json:"taker_buy_mad"`
	TakerBuyLatest   float64 `json:"taker_buy_latest"`
	TakerBuyAnomalyZ float64 `json:"taker_buy_anomaly_z"`
	TakerBuyAnomaly  bool    `json:"taker_buy_anomaly"`

	BidSupportStreak  int     `json:"bid_support_streak"`
	BidSupportPct     float64 `json:"bid_support_pct"`
	BidWallPersistent bool    `json:"bid_wall_persistent"`
	CoreSignalCount   int     `json:"core_signal_count"`
	SignalPersistence int     `json:"signal_persistence"`

	FundingLatest    float64  `json:"funding_latest"`
	FundingFavorable bool     `json:"funding_favorable"`
	BasisLatest      float64  `json:"basis_latest"`
	BasisNegative    bool     `json:"basis_negative"`
	FootprintScore   float64  `json:"footprint_score"`
	Verdict          string   `json:"verdict"`
	Reasons          []string `json:"reasons,omitempty"`
}

// AnalyzeMMFootprint expects newest-first snapshots. Thresholds adapt to each
// symbol using robust median/MAD rather than fixed cross-asset values.
func AnalyzeMMFootprint(snapshots []Snapshot) MMFootprintSignal {
	return AnalyzeMMFootprintWithThreshold(snapshots, DefaultTakerAnomalyZ)
}

func AnalyzeMMFootprintWithThreshold(snapshots []Snapshot, takerAnomalyZ float64) MMFootprintSignal {
	if takerAnomalyZ < MinTakerAnomalyZ || takerAnomalyZ > MaxTakerAnomalyZ {
		takerAnomalyZ = DefaultTakerAnomalyZ
	}
	sig := MMFootprintSignal{Verdict: "NO_SIGNAL"}
	if len(snapshots) < 3 {
		sig.Reasons = []string{"insufficient snapshot history (need >= 3)"}
		return sig
	}
	// Work on a copy sorted newest-first and discard duplicate/invalid times.
	s := append([]Snapshot(nil), snapshots...)
	sort.SliceStable(s, func(i, j int) bool { return s[i].Timestamp.After(s[j].Timestamp) })
	clean := make([]Snapshot, 0, len(s))
	for _, x := range s {
		if x.Timestamp.IsZero() {
			continue
		}
		if len(clean) > 0 && x.Timestamp.Equal(clean[len(clean)-1].Timestamp) {
			continue
		}
		clean = append(clean, x)
	}
	if len(clean) < 3 {
		sig.Reasons = []string{"insufficient unique timestamp history"}
		return sig
	}
	s = clean
	n := len(s)
	sig.SnapshotCount = n
	duration := s[0].Timestamp.Sub(s[n-1].Timestamp)
	sig.HistoryMinutes = duration.Minutes()
	intervals := make([]float64, 0, n-1)
	for i := 0; i < n-1; i++ {
		d := s[i].Timestamp.Sub(s[i+1].Timestamp).Minutes()
		if d > 0 {
			intervals = append(intervals, d)
		}
	}
	medianInterval := median(intervals)
	if medianInterval > 0 && duration > 0 {
		expected := duration.Minutes()/medianInterval + 1
		sig.CoveragePct = math.Min(1, float64(n)/expected)
	}
	sig.Fresh = time.Since(s[0].Timestamp) <= time.Duration(math.Max(15, medianInterval*2))*time.Minute
	sig.DataQuality = mmClamp01(0.35*math.Min(1, float64(n)/8) + 0.35*math.Min(1, sig.HistoryMinutes/60) + 0.30*sig.CoveragePct)

	sig.CVDLatest, sig.CVDEarliest = s[0].SpotFlow.CVDQuoteUSDT, s[n-1].SpotFlow.CVDQuoteUSDT
	sig.CVDDelta = sig.CVDLatest - sig.CVDEarliest
	if sig.HistoryMinutes > 0 {
		sig.CVDSlope = sig.CVDDelta / sig.HistoryMinutes
	}
	volume := 0.0
	for _, x := range s {
		volume += math.Max(0, x.SpotFlow.QuoteVolumeUSDT)
	}
	if volume > 0 {
		sig.NormalizedCVDSlope = sig.CVDDelta / volume
	}
	sig.PriceLatest, sig.PriceEarliest = s[0].OrderBook.BestBid, s[n-1].OrderBook.BestBid
	if sig.PriceLatest > 0 && sig.PriceEarliest > 0 {
		sig.PriceDeltaPct = (sig.PriceLatest - sig.PriceEarliest) / sig.PriceEarliest * 100
	}
	// Normalized positive CVD with non-pumping price is the first core signal.
	sig.CVDPriceDivergence = sig.PriceDeltaPct >= -2 && sig.PriceDeltaPct <= 2 && sig.NormalizedCVDSlope > 0.0005 && sig.CVDSlope > 0

	ratios := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		if s[i].SpotFlow.TakerBuyRatio > 0 {
			ratios = append(ratios, s[i].SpotFlow.TakerBuyRatio)
		}
	}
	sig.TakerBuyBaseline = median(ratios)
	dev := make([]float64, 0, len(ratios))
	for _, v := range ratios {
		dev = append(dev, math.Abs(v-sig.TakerBuyBaseline))
	}
	sig.TakerBuyMAD = median(dev)
	sig.TakerBuyLatest = s[0].SpotFlow.TakerBuyRatio
	scale := math.Max(0.005, 1.4826*sig.TakerBuyMAD)
	sig.TakerBuyAnomalyZ = (sig.TakerBuyLatest - sig.TakerBuyBaseline) / scale
	sig.TakerBuyAnomaly = sig.TakerBuyAnomalyZ >= takerAnomalyZ && sig.PriceDeltaPct < 2

	for i, x := range s {
		if x.Signals.OrderBookBias == "BID_SUPPORT" {
			if i == sig.BidSupportStreak {
				sig.BidSupportStreak++
			}
		}
	}
	bidCount := 0
	for _, x := range s {
		if x.Signals.OrderBookBias == "BID_SUPPORT" {
			bidCount++
		}
	}
	sig.BidSupportPct = float64(bidCount) / float64(n)
	sig.BidWallPersistent = sig.BidSupportStreak >= 3 && sig.BidSupportPct >= 0.30
	sig.CurrentAskPressure = s[0].Signals.OrderBookBias == "ASK_PRESSURE"

	if sig.CVDPriceDivergence {
		sig.CoreSignalCount++
		sig.Reasons = append(sig.Reasons, "time/volume-normalized CVD improves while price remains contained")
	}
	if sig.TakerBuyAnomaly {
		sig.CoreSignalCount++
		sig.Reasons = append(sig.Reasons, "taker buy ratio exceeds adaptive median/MAD threshold")
	}
	if sig.BidWallPersistent {
		sig.CoreSignalCount++
		sig.Reasons = append(sig.Reasons, "bid support persists across recent snapshots")
	}
	// Persistence: count latest intervals with improving CVD or bid support.
	for i := 0; i < n-1; i++ {
		improving := s[i].SpotFlow.CVDQuoteUSDT > s[i+1].SpotFlow.CVDQuoteUSDT || s[i].Signals.OrderBookBias == "BID_SUPPORT"
		if !improving {
			break
		}
		sig.SignalPersistence++
	}

	sig.FundingLatest = s[0].Futures.FundingRate
	sig.FundingFavorable = sig.FundingLatest <= 0.0001
	sig.BasisLatest = s[0].Futures.BasisPct
	sig.BasisNegative = sig.BasisLatest < -0.02

	coreScore := 0.0
	if sig.CVDPriceDivergence {
		coreScore += 0.40
	}
	if sig.TakerBuyAnomaly {
		coreScore += 0.30
	}
	if sig.BidWallPersistent {
		coreScore += 0.30
	}
	context := 0.0
	if sig.CoreSignalCount > 0 && sig.FundingFavorable {
		context += 0.04
	}
	if sig.CoreSignalCount > 0 && sig.BasisNegative {
		context += 0.04
	}
	sig.FootprintScore = mmClamp01((coreScore + context) * sig.DataQuality)

	switch {
	case sig.CoreSignalCount >= 2 && sig.SignalPersistence >= 2 && sig.DataQuality >= 0.65 && sig.HistoryMinutes >= 60 && !sig.CurrentAskPressure:
		sig.Verdict = "MM_ACCUMULATING"
	case sig.CoreSignalCount >= 1 && sig.DataQuality >= 0.45:
		sig.Verdict = "POSSIBLE_ACCUMULATION"
	case sig.CoreSignalCount >= 1:
		sig.Verdict = "WATCH"
	default:
		sig.Verdict = "NO_SIGNAL"
	}
	return sig
}

func AnalyzeMMFootprintMulti(historyBySymbol map[string][]Snapshot) map[string]MMFootprintSignal {
	return AnalyzeMMFootprintMultiCalibrated(historyBySymbol, nil)
}

func AnalyzeMMFootprintMultiCalibrated(historyBySymbol map[string][]Snapshot, thresholds map[string]float64) map[string]MMFootprintSignal {
	out := make(map[string]MMFootprintSignal, len(historyBySymbol))
	for sym, snaps := range historyBySymbol {
		threshold := DefaultTakerAnomalyZ
		if v := thresholds[strings.ToUpper(sym)]; v >= MinTakerAnomalyZ && v <= MaxTakerAnomalyZ {
			threshold = v
		}
		out[sym] = AnalyzeMMFootprintWithThreshold(snaps, threshold)
	}
	return out
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	m := len(x) / 2
	if len(x)%2 == 1 {
		return x[m]
	}
	return (x[m-1] + x[m]) / 2
}

func mmClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
