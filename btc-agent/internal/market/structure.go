package market

type Structure struct {
	Label            string `json:"label"`
	LowerLowCount    int    `json:"lower_low_count"`
	HigherHighCount  int    `json:"higher_high_count"`
	BreakDown        bool   `json:"break_down"`
	BreakUp          bool   `json:"break_up"`
	LiquidityReclaim bool   `json:"liquidity_reclaim"`
}

func AnalyzeStructure(c []Candle) Structure {
	if len(c) < 10 {
		return Structure{Label: "UNKNOWN"}
	}
	lows := recentSwingLows(c, 5)
	highs := recentSwingHighs(c, 5)
	s := Structure{Label: "RANGE"}
	if len(lows) >= 3 {
		for i := 1; i < len(lows); i++ {
			if lows[i] < lows[i-1] {
				s.LowerLowCount++
			}
		}
	}
	if len(highs) >= 3 {
		for i := 1; i < len(highs); i++ {
			if highs[i] > highs[i-1] {
				s.HigherHighCount++
			}
		}
	}
	last := c[len(c)-1]
	if len(lows) > 0 && last.Close < lows[len(lows)-1] {
		s.BreakDown = true
	}
	if len(highs) > 0 && last.Close > highs[len(highs)-1] {
		s.BreakUp = true
	}
	if len(lows) > 0 && last.Low < lows[len(lows)-1] && last.Close > lows[len(lows)-1] {
		s.LiquidityReclaim = true
	}
	if s.LowerLowCount >= 2 {
		s.Label = "LL"
	} else if s.HigherHighCount >= 2 {
		s.Label = "HH"
	}
	return s
}

func recentSwingLows(c []Candle, n int) []float64 {
	out := []float64{}
	for i := 2; i < len(c)-2; i++ {
		if c[i].Low < c[i-1].Low && c[i].Low < c[i-2].Low && c[i].Low < c[i+1].Low && c[i].Low < c[i+2].Low {
			out = append(out, c[i].Low)
		}
	}
	if len(out) > n {
		return out[len(out)-n:]
	}
	return out
}
func recentSwingHighs(c []Candle, n int) []float64 {
	out := []float64{}
	for i := 2; i < len(c)-2; i++ {
		if c[i].High > c[i-1].High && c[i].High > c[i-2].High && c[i].High > c[i+1].High && c[i].High > c[i+2].High {
			out = append(out, c[i].High)
		}
	}
	if len(out) > n {
		return out[len(out)-n:]
	}
	return out
}
