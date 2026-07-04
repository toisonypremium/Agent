package indicators

type MACDPoint struct{ MACD, Signal, Hist float64 }

func MACD(closes []float64, fast, slow, signal int) []MACDPoint {
	out := make([]MACDPoint, len(closes))
	if len(closes) == 0 {
		return out
	}
	f, s := EMA(closes, fast), EMA(closes, slow)
	line := make([]float64, len(closes))
	for i := range closes {
		line[i] = f[i] - s[i]
	}
	sig := EMA(line, signal)
	for i := range closes {
		out[i] = MACDPoint{MACD: line[i], Signal: sig[i], Hist: line[i] - sig[i]}
	}
	return out
}
