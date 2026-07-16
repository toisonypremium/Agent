package microstructure

import "math"

// MMFootprintSignal tổng hợp dấu vết MM (market maker) từ lịch sử nhiều snapshot.
// Phát hiện pattern: giá đi ngang/nhích nhẹ trong khi sell pressure liên tục
// bị hấp thụ — dấu hiệu MM đang gom hàng âm thầm trước khi giá bứt phá.
type MMFootprintSignal struct {
	SnapshotCount int `json:"snapshot_count"`

	// CVD trend qua N snapshot
	CVDLatest          float64 `json:"cvd_latest"`
	CVDEarliest        float64 `json:"cvd_earliest"`
	CVDDelta           float64 `json:"cvd_delta"`
	CVDSlope           float64 `json:"cvd_slope"` // > 0 = CVD tăng dần (buy absorption)

	// Price vs CVD divergence
	PriceLatest        float64 `json:"price_latest"`
	PriceEarliest      float64 `json:"price_earliest"`
	PriceDeltaPct      float64 `json:"price_delta_pct"`
	CVDPriceDivergence bool    `json:"cvd_price_divergence"` // price >= -2% & CVDSlope > 0

	// Taker buy ratio anomaly
	TakerBuyBaseline float64 `json:"taker_buy_baseline"`
	TakerBuyLatest   float64 `json:"taker_buy_latest"`
	TakerBuyAnomaly  bool    `json:"taker_buy_anomaly"` // ratio cao bất thường khi price flat

	// Bid wall persistence
	BidSupportStreak  int     `json:"bid_support_streak"`
	BidSupportPct     float64 `json:"bid_support_pct"`
	BidWallPersistent bool    `json:"bid_wall_persistent"` // streak >= 4

	// Funding & basis
	FundingLatest    float64 `json:"funding_latest"`
	FundingFavorable bool    `json:"funding_favorable"` // <= 0.0001
	BasisLatest      float64 `json:"basis_latest"`
	BasisNegative    bool    `json:"basis_negative"` // perp discount

	// Tổng hợp
	FootprintScore float64  `json:"footprint_score"`
	Verdict        string   `json:"verdict"` // MM_ACCUMULATING | POSSIBLE_ACCUMULATION | WATCH | NO_SIGNAL
	Reasons        []string `json:"reasons,omitempty"`
}

// AnalyzeMMFootprint nhận N snapshot lịch sử của một symbol (newest first).
// Cần >= 3 snapshot.
func AnalyzeMMFootprint(snapshots []Snapshot) MMFootprintSignal {
	sig := MMFootprintSignal{Verdict: "NO_SIGNAL"}
	n := len(snapshots)
	if n < 3 {
		sig.Reasons = append(sig.Reasons, "insufficient snapshot history (need >= 3)")
		return sig
	}
	sig.SnapshotCount = n

	// CVD: newest = snapshots[0], oldest = snapshots[n-1]
	sig.CVDLatest = snapshots[0].SpotFlow.CVDQuoteUSDT
	sig.CVDEarliest = snapshots[n-1].SpotFlow.CVDQuoteUSDT
	sig.CVDDelta = sig.CVDLatest - sig.CVDEarliest

	// slope: trung bình bước thay đổi CVD theo chiều thời gian (mới→cũ đảo chiều)
	slopeSum := 0.0
	for i := 0; i < n-1; i++ {
		slopeSum += snapshots[i].SpotFlow.CVDQuoteUSDT - snapshots[i+1].SpotFlow.CVDQuoteUSDT
	}
	sig.CVDSlope = slopeSum / float64(n-1)

	// Price: dùng best bid
	sig.PriceLatest = snapshots[0].OrderBook.BestBid
	for i := n - 1; i >= 0; i-- {
		if snapshots[i].OrderBook.BestBid > 0 {
			sig.PriceEarliest = snapshots[i].OrderBook.BestBid
			break
		}
	}
	if sig.PriceEarliest > 0 && sig.PriceLatest > 0 {
		sig.PriceDeltaPct = (sig.PriceLatest - sig.PriceEarliest) / sig.PriceEarliest * 100
	}
	// Divergence: giá không giảm nhiều nhưng CVD slope tăng → sell bị hấp thụ
	sig.CVDPriceDivergence = sig.PriceDeltaPct >= -2.0 && sig.CVDSlope > 0

	// Taker buy baseline
	var takerSum float64
	validTaker := 0
	for _, s := range snapshots {
		if s.SpotFlow.TakerBuyRatio > 0 {
			takerSum += s.SpotFlow.TakerBuyRatio
			validTaker++
		}
	}
	if validTaker > 0 {
		sig.TakerBuyBaseline = takerSum / float64(validTaker)
	}
	sig.TakerBuyLatest = snapshots[0].SpotFlow.TakerBuyRatio
	// Anomaly: taker buy cao bất thường khi giá không pumping (< 3%)
	sig.TakerBuyAnomaly = sig.TakerBuyLatest > sig.TakerBuyBaseline+0.04 && sig.PriceDeltaPct < 3.0

	// Bid wall persistence (streak từ snapshot mới nhất)
	streak := 0
	bidCount := 0
	streakBroken := false
	for _, s := range snapshots {
		if s.Signals.OrderBookBias == "BID_SUPPORT" {
			bidCount++
			if !streakBroken {
				streak++
			}
		} else {
			streakBroken = true
		}
	}
	sig.BidSupportStreak = streak
	if n > 0 {
		sig.BidSupportPct = float64(bidCount) / float64(n)
	}
	sig.BidWallPersistent = sig.BidSupportStreak >= 4

	// Funding & basis
	sig.FundingLatest = snapshots[0].Futures.FundingRate
	sig.FundingFavorable = sig.FundingLatest <= 0.0001
	sig.BasisLatest = snapshots[0].Futures.BasisPct
	sig.BasisNegative = sig.BasisLatest < -0.02

	// Score
	score := 0.0
	var reasons []string

	if sig.CVDPriceDivergence {
		score += 0.35
		reasons = append(reasons, "CVD slope rising while price flat → sell pressure being absorbed")
	}
	if sig.TakerBuyAnomaly {
		score += 0.25
		reasons = append(reasons, "taker buy ratio above baseline +4% while price not pumping")
	}
	if sig.BidWallPersistent {
		score += 0.20
		reasons = append(reasons, "bid wall held >= 4 consecutive snapshots")
	} else if sig.BidSupportStreak >= 2 {
		score += 0.08
		reasons = append(reasons, "bid support present >= 2 consecutive snapshots")
	}
	if sig.FundingFavorable {
		score += 0.10
		reasons = append(reasons, "funding neutral/negative → smart money not overcrowded long")
	}
	if sig.BasisNegative {
		score += 0.10
		reasons = append(reasons, "basis negative → perp discount, spot preferred by smart money")
	}
	// Penalty: CVD tiếp tục xấu mạnh
	if sig.CVDSlope < 0 && math.Abs(sig.CVDSlope) > 500000 {
		score -= 0.15
		reasons = append(reasons, "CVD slope still declining sharply → sell not fully absorbed")
	}

	sig.FootprintScore = mmClamp01(score)
	sig.Reasons = reasons

	switch {
	case sig.FootprintScore >= 0.65 && sig.CVDPriceDivergence && sig.TakerBuyAnomaly:
		sig.Verdict = "MM_ACCUMULATING"
	case sig.FootprintScore >= 0.40 && (sig.CVDPriceDivergence || sig.TakerBuyAnomaly):
		sig.Verdict = "POSSIBLE_ACCUMULATION"
	case sig.FootprintScore >= 0.20:
		sig.Verdict = "WATCH"
	default:
		sig.Verdict = "NO_SIGNAL"
	}
	return sig
}

// AnalyzeMMFootprintMulti phân tích nhiều symbol, trả về map symbol → footprint.
func AnalyzeMMFootprintMulti(historyBySymbol map[string][]Snapshot) map[string]MMFootprintSignal {
	out := make(map[string]MMFootprintSignal, len(historyBySymbol))
	for sym, snaps := range historyBySymbol {
		out[sym] = AnalyzeMMFootprint(snaps)
	}
	return out
}

func mmClamp01(v float64) float64 {
	if v < 0 { return 0 }
	if v > 1 { return 1 }
	return v
}
