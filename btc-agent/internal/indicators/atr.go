package indicators

func ATR(highs, lows, closes []float64, period int) []float64 {
	out := make([]float64, len(closes))
	if period <= 0 || len(closes) < 2 || len(highs) != len(closes) || len(lows) != len(closes) {
		return out
	}
	trs := make([]float64, len(closes))
	for i := 1; i < len(closes); i++ {
		h, l, pc := highs[i], lows[i], closes[i-1]
		tr := h - l
		if v := abs(h - pc); v > tr {
			tr = v
		}
		if v := abs(l - pc); v > tr {
			tr = v
		}
		trs[i] = tr
	}
	sum := 0.0
	for i := 1; i < len(closes); i++ {
		sum += trs[i]
		if i > period {
			sum -= trs[i-period]
		}
		if i >= period {
			out[i] = sum / float64(period)
		}
	}
	return out
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
