package liquidity

import (
	"fmt"
	"math"

	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

const (
	GradeA       = "A"
	GradeB       = "B"
	GradeC       = "C"
	GradeD       = "D"
	GradeUnknown = "UNKNOWN_PROXY"
)

type Quality struct {
	Enabled                 bool     `json:"enabled"`
	Symbol                  string   `json:"symbol"`
	Pass                    bool     `json:"pass"`
	Score                   float64  `json:"score"`
	Grade                   string   `json:"grade"`
	SpreadBps               float64  `json:"spread_bps,omitempty"`
	BidDepth1PctUSDT        float64  `json:"bid_depth_1pct_usdt,omitempty"`
	AskDepth1PctUSDT        float64  `json:"ask_depth_1pct_usdt,omitempty"`
	Avg5mQuoteVolume        float64  `json:"avg_5m_quote_volume,omitempty"`
	AvgQuoteVolumeProxy     float64  `json:"avg_quote_volume_proxy,omitempty"`
	EstimatedSlippageBps    float64  `json:"estimated_slippage_bps,omitempty"`
	OrderToAvgVolumePct     float64  `json:"order_to_avg_volume_pct,omitempty"`
	RecentRangePct          float64  `json:"recent_range_pct,omitempty"`
	MaxOrderToAvgVolumePct  float64  `json:"max_order_to_avg_volume_pct,omitempty"`
	MinBidDepthToOrderRatio float64  `json:"min_bid_depth_to_order_ratio,omitempty"`
	Reasons                 []string `json:"reasons,omitempty"`
}

type OrderBookSnapshot struct {
	BestBid          float64 `json:"best_bid,omitempty"`
	BestAsk          float64 `json:"best_ask,omitempty"`
	BidDepth1PctUSDT float64 `json:"bid_depth_1pct_usdt,omitempty"`
	AskDepth1PctUSDT float64 `json:"ask_depth_1pct_usdt,omitempty"`
}

func EvaluateCandleProxy(cfg config.Config, symbol string, candles []market.Candle, orderNotional float64) Quality {
	q := Quality{Enabled: true, Symbol: symbol, Pass: true, Grade: GradeUnknown}
	maxOrderPct := cfg.Live.MaxOrderToAvg5mQuoteVolumePct
	if maxOrderPct <= 0 {
		maxOrderPct = 0.02
	}
	q.MaxOrderToAvgVolumePct = maxOrderPct
	q.MinBidDepthToOrderRatio = minDepthRatio(cfg)
	if orderNotional <= 0 {
		q.Pass = false
		q.Grade = GradeD
		q.Reasons = append(q.Reasons, "liquidity gate: order notional invalid")
		q.Score = 0
		return q
	}
	if len(candles) < 20 {
		q.Pass = false
		q.Grade = GradeD
		q.Reasons = append(q.Reasons, "liquidity gate: chưa đủ dữ liệu candle proxy")
		q.Score = 0
		return q
	}
	avgQuote := averageQuoteVolume(candles, 20)
	q.AvgQuoteVolumeProxy = avgQuote
	q.Avg5mQuoteVolume = avgQuote / 288.0
	if q.Avg5mQuoteVolume > 0 {
		q.OrderToAvgVolumePct = orderNotional / q.Avg5mQuoteVolume
	}
	rangePct := averageRangePct(candles, 10)
	q.RecentRangePct = rangePct
	volumeScore := 0.0
	if q.OrderToAvgVolumePct > 0 {
		volumeScore = clamp01(maxOrderPct / q.OrderToAvgVolumePct)
	}
	stabilityScore := clamp01(0.08 / math.Max(rangePct, 1e-9))
	q.Score = math.Round((volumeScore*70+stabilityScore*30)*10) / 10
	if q.OrderToAvgVolumePct > maxOrderPct {
		q.Pass = false
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity gate: order %.2f%% avg 5m quote volume > %.2f%%", q.OrderToAvgVolumePct*100, maxOrderPct*100))
	}
	if rangePct > 0.12 {
		q.Pass = false
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity gate: range nhiễu %.2f%% > 12.00%%", rangePct*100))
	}
	q.Grade = grade(q.Score, q.Pass)
	if len(q.Reasons) == 0 {
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity proxy OK: order %.2f%% avg 5m volume, range %.2f%%", q.OrderToAvgVolumePct*100, rangePct*100))
	}
	return q
}

func ApplyOrderBook(cfg config.Config, q Quality, book OrderBookSnapshot, orderNotional float64) Quality {
	if !q.Enabled {
		q.Enabled = true
	}
	if book.BestBid > 0 && book.BestAsk > 0 && book.BestAsk >= book.BestBid {
		mid := (book.BestBid + book.BestAsk) / 2
		q.SpreadBps = (book.BestAsk - book.BestBid) / mid * 10000
	}
	q.BidDepth1PctUSDT = book.BidDepth1PctUSDT
	q.AskDepth1PctUSDT = book.AskDepth1PctUSDT
	maxSpread := cfg.Live.MaxSpreadBps
	if maxSpread <= 0 {
		maxSpread = 15
	}
	maxSlip := cfg.Live.MaxSlippageBps
	if maxSlip <= 0 {
		maxSlip = 30
	}
	depthRatio := minDepthRatio(cfg)
	if orderNotional > 0 && q.BidDepth1PctUSDT > 0 {
		q.EstimatedSlippageBps = orderNotional / q.BidDepth1PctUSDT * 10000
	}
	if q.SpreadBps > maxSpread {
		q.Pass = false
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity gate: spread %.2fbps > %.2fbps", q.SpreadBps, maxSpread))
	}
	if orderNotional > 0 && q.BidDepth1PctUSDT > 0 && q.BidDepth1PctUSDT < orderNotional*depthRatio {
		q.Pass = false
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity gate: bid depth %.2f < %.1fx order", q.BidDepth1PctUSDT, depthRatio))
	}
	if q.EstimatedSlippageBps > maxSlip {
		q.Pass = false
		q.Reasons = append(q.Reasons, fmt.Sprintf("liquidity gate: slippage %.2fbps > %.2fbps", q.EstimatedSlippageBps, maxSlip))
	}
	q.Grade = grade(q.Score, q.Pass)
	return q
}

func minDepthRatio(cfg config.Config) float64 {
	v := cfg.Live.MinBidDepthToOrderRatio
	if v <= 0 {
		return 20
	}
	return v
}

func averageQuoteVolume(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	for _, x := range c[start:] {
		sum += x.Close * x.Volume
	}
	return sum / float64(n)
}

func averageRangePct(c []market.Candle, n int) float64 {
	if len(c) == 0 || n <= 0 {
		return 0
	}
	if n > len(c) {
		n = len(c)
	}
	start := len(c) - n
	sum := 0.0
	count := 0
	for _, x := range c[start:] {
		if x.Close <= 0 {
			continue
		}
		sum += (x.High - x.Low) / x.Close
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func grade(score float64, pass bool) string {
	if !pass {
		return GradeD
	}
	switch {
	case score >= 80:
		return GradeA
	case score >= 60:
		return GradeB
	case score >= 40:
		return GradeC
	default:
		return GradeD
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
