package microstructure

import (
	"fmt"
	"math"
	"sort"

	"btc-agent/internal/market"
)

// LiquidationProxy is a report-only estimate derived from public OHLCV and
// open-interest observations. It is not an exchange liquidation feed.
type ResearchDiagnostics struct {
	LiquidationProxy LiquidationProxy `json:"liquidation_proxy"`
	AnchoredVWAP     float64          `json:"anchored_vwap"`
	VolumeProfile    VolumeProfile    `json:"volume_profile"`
	Warnings         []string         `json:"warnings,omitempty"`
}

type LiquidationProxy struct {
	Status             string  `json:"status"`
	Direction          string  `json:"direction"`
	PriceReturnPct     float64 `json:"price_return_pct"`
	OpenInterestChange float64 `json:"open_interest_change"`
	Confidence         float64 `json:"confidence"`
	Reason             string  `json:"reason"`
}

// EstimateLiquidationProxy identifies possible forced-flow pressure from a
// price move paired with an open-interest change. It never grants execution
// authority and returns BLOCKED when observations are incomplete.
func EstimateLiquidationProxy(previousPrice, currentPrice, previousOpenInterest, currentOpenInterest float64) LiquidationProxy {
	out := LiquidationProxy{Status: "BLOCKED", Direction: "UNKNOWN"}
	if previousPrice <= 0 || currentPrice <= 0 || previousOpenInterest <= 0 || currentOpenInterest <= 0 ||
		math.IsNaN(previousPrice) || math.IsNaN(currentPrice) || math.IsNaN(previousOpenInterest) || math.IsNaN(currentOpenInterest) ||
		math.IsInf(previousPrice, 0) || math.IsInf(currentPrice, 0) || math.IsInf(previousOpenInterest, 0) || math.IsInf(currentOpenInterest, 0) {
		out.Reason = "insufficient finite price/open-interest observations"
		return out
	}
	priceChange := (currentPrice - previousPrice) / previousPrice
	oiChange := (currentOpenInterest - previousOpenInterest) / previousOpenInterest
	out.PriceReturnPct = priceChange * 100
	out.OpenInterestChange = oiChange
	// Falling price with falling OI suggests long liquidation pressure; rising
	// price with falling OI suggests short-covering pressure. Rising OI is not
	// liquidation evidence by itself, so keep the result neutral.
	switch {
	case priceChange < -0.002 && oiChange < -0.005:
		out.Direction, out.Status, out.Reason = "LONG_LIQUIDATION_PRESSURE", "PROXY", "price and open interest declined together"
	case priceChange > 0.002 && oiChange < -0.005:
		out.Direction, out.Status, out.Reason = "SHORT_COVER_PRESSURE", "PROXY", "price rose while open interest declined"
	case priceChange > 0.002 && oiChange > 0.005:
		out.Direction, out.Status, out.Reason = "SHORT_BUILDUP_OR_FRESH_DEMAND", "PROXY", "price and open interest rose together; liquidation not established"
	case priceChange < -0.002 && oiChange > 0.005:
		out.Direction, out.Status, out.Reason = "LONG_UNWIND_RISK", "PROXY", "price declined while open interest rose; liquidation not established"
	default:
		out.Direction, out.Status, out.Reason = "NEUTRAL", "PROXY", "price/open-interest changes below proxy thresholds"
	}
	out.Confidence = math.Min(1, math.Abs(priceChange)*100+math.Abs(oiChange)*20)
	return out
}

// AnchoredVWAP computes a volume-weighted average using candles at or after
// anchor. It is research-only and returns zero when data is invalid.
func AnchoredVWAP(candles []market.Candle, anchor int) (float64, error) {
	if anchor < 0 || anchor >= len(candles) {
		return 0, fmt.Errorf("anchor index %d outside candle range", anchor)
	}
	var priceVolume, volume float64
	for _, candle := range candles[anchor:] {
		if candle.Close <= 0 || candle.Volume < 0 || math.IsNaN(candle.Close) || math.IsNaN(candle.Volume) || math.IsInf(candle.Close, 0) || math.IsInf(candle.Volume, 0) {
			return 0, fmt.Errorf("invalid candle for anchored VWAP")
		}
		if candle.Volume == 0 {
			continue
		}
		priceVolume += candle.Close * candle.Volume
		volume += candle.Volume
	}
	if volume <= 0 {
		return 0, fmt.Errorf("anchored VWAP has no positive volume")
	}
	return priceVolume / volume, nil
}

type VolumeProfileLevel struct {
	Price       float64 `json:"price"`
	Volume      float64 `json:"volume"`
	VolumeShare float64 `json:"volume_share"`
}

type VolumeProfile struct {
	Status      string               `json:"status"`
	Low         float64              `json:"low"`
	High        float64              `json:"high"`
	POC         float64              `json:"poc"`
	Levels      []VolumeProfileLevel `json:"levels,omitempty"`
	TotalVolume float64              `json:"total_volume"`
}

// BuildVolumeProfile bins candle volume by typical price. It is diagnostic
// only; the result never changes a plan or creates an order.
func BuildVolumeProfile(candles []market.Candle, bins int) (VolumeProfile, error) {
	out := VolumeProfile{Status: "BLOCKED"}
	if len(candles) == 0 || bins <= 0 {
		return out, fmt.Errorf("volume profile requires candles and positive bins")
	}
	low, high := math.Inf(1), math.Inf(-1)
	for _, candle := range candles {
		if candle.Low <= 0 || candle.High < candle.Low || candle.Close <= 0 || candle.Volume < 0 ||
			math.IsNaN(candle.Low) || math.IsNaN(candle.High) || math.IsNaN(candle.Close) || math.IsNaN(candle.Volume) ||
			math.IsInf(candle.Low, 0) || math.IsInf(candle.High, 0) || math.IsInf(candle.Close, 0) || math.IsInf(candle.Volume, 0) {
			return out, fmt.Errorf("invalid candle for volume profile")
		}
		low = math.Min(low, candle.Low)
		high = math.Max(high, candle.High)
	}
	if high <= 0 || low > high {
		return out, fmt.Errorf("invalid profile range")
	}
	if high == low {
		for _, candle := range candles {
			out.TotalVolume += candle.Volume
		}
		if out.TotalVolume <= 0 {
			return out, fmt.Errorf("volume profile has no positive volume")
		}
		out.Low, out.High, out.POC, out.Status = low, high, low, "OK"
		out.Levels = []VolumeProfileLevel{{Price: low, Volume: out.TotalVolume, VolumeShare: 1}}
		return out, nil
	}
	width := (high - low) / float64(bins)
	volumes := make([]float64, bins)
	for _, candle := range candles {
		price := (candle.High + candle.Low + candle.Close) / 3
		idx := int((price - low) / width)
		if idx < 0 {
			idx = 0
		}
		if idx >= bins {
			idx = bins - 1
		}
		volumes[idx] += candle.Volume
		out.TotalVolume += candle.Volume
	}
	out.Low, out.High, out.Status = low, high, "OK"
	if out.TotalVolume <= 0 {
		return out, fmt.Errorf("volume profile has no positive volume")
	}
	poc := 0
	for i, volume := range volumes {
		if volume > volumes[poc] {
			poc = i
		}
		levelPrice := low + (float64(i)+0.5)*width
		out.Levels = append(out.Levels, VolumeProfileLevel{Price: levelPrice, Volume: volume, VolumeShare: volume / out.TotalVolume})
	}
	out.POC = out.Levels[poc].Price
	sort.Slice(out.Levels, func(i, j int) bool { return out.Levels[i].Price < out.Levels[j].Price })
	return out, nil
}
