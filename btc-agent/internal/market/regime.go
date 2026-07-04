package market

import "btc-agent/internal/indicators"

type FrameSignal struct {
	Bias       string    `json:"bias"`
	TrendScore float64   `json:"trend_score"`
	EMA20      float64   `json:"ema20"`
	EMA50      float64   `json:"ema50"`
	EMA200     float64   `json:"ema200"`
	RSI14      float64   `json:"rsi14"`
	ATR14      float64   `json:"atr14"`
	VolumeMA20 float64   `json:"volume_ma20"`
	Structure  Structure `json:"structure"`
}

func Frame(c []Candle) FrameSignal {
	if len(c) < 50 {
		return FrameSignal{Bias: "UNKNOWN"}
	}
	cl := make([]float64, len(c))
	highs := make([]float64, len(c))
	lows := make([]float64, len(c))
	volumes := make([]float64, len(c))
	for i, x := range c {
		cl[i] = x.Close
		highs[i] = x.High
		lows[i] = x.Low
		volumes[i] = x.Volume
	}
	e20, e50, e200 := indicators.EMA(cl, 20), indicators.EMA(cl, 50), indicators.EMA(cl, 200)
	rsi := indicators.RSI(cl, 14)
	atr := indicators.ATR(highs, lows, cl, 14)
	vma := indicators.VolumeMA(volumes, 20)
	last := cl[len(cl)-1]
	score := 0.0
	if last > indicators.Last(e20) {
		score += 25
	}
	if indicators.Last(e20) > indicators.Last(e50) {
		score += 25
	}
	if last > indicators.Last(e200) {
		score += 20
	}
	if indicators.Last(rsi) > 50 {
		score += 15
	}
	st := AnalyzeStructure(c)
	if st.BreakUp {
		score += 15
	}
	if st.BreakDown {
		score -= 25
	}
	bias := "RANGE"
	if score >= 70 {
		bias = "BULLISH"
	} else if score <= 30 {
		bias = "BEARISH"
	}
	return FrameSignal{Bias: bias, TrendScore: score, EMA20: indicators.Last(e20), EMA50: indicators.Last(e50), EMA200: indicators.Last(e200), RSI14: indicators.Last(rsi), ATR14: indicators.Last(atr), VolumeMA20: indicators.Last(vma), Structure: st}
}
