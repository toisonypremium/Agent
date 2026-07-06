package market

func RangeZone(c []Candle, lookback int) (Zone, Zone) {
	if len(c) == 0 {
		return Zone{}, Zone{}
	}
	if lookback > len(c) {
		lookback = len(c)
	}
	start := len(c) - lookback
	hi, lo := c[start].High, c[start].Low
	for _, x := range c[start:] {
		if x.High > hi {
			hi = x.High
		}
		if x.Low < lo {
			lo = x.Low
		}
	}
	width := (hi - lo) * 0.08
	if width <= 0 {
		width = lo * 0.005
		if width <= 0 {
			width = 1
		}
	}
	return Zone{Low: lo, High: lo + width, Name: "support"}, Zone{Low: hi - width, High: hi, Name: "resistance"}
}
func DeepSupport(c []Candle, lookback int) Zone {
	if len(c) == 0 {
		return Zone{}
	}
	if lookback > len(c) {
		lookback = len(c)
	}
	start := len(c) - lookback
	lo := c[start].Low
	for _, x := range c[start:] {
		if x.Low < lo {
			lo = x.Low
		}
	}
	return Zone{Low: lo * 0.97, High: lo * 1.03, Name: "deep_support"}
}
func AccumulationZone(support Zone, stress float64) Zone {
	return ActiveAccumulationZone(support)
}

func ActiveAccumulationZone(support Zone) Zone {
	if !support.Valid() {
		return Zone{}
	}
	return Zone{Low: support.Low, High: support.High, Name: "active_accumulation"}
}

func MacroAccumulationZone(support Zone, stress float64) Zone {
	if stress > 0 && support.Valid() && stress < support.Low {
		return Zone{Low: stress * 0.98, High: support.High, Name: "macro_accumulation"}
	}
	if support.Valid() {
		return Zone{Low: support.Low, High: support.High, Name: "macro_accumulation"}
	}
	return Zone{}
}

func ZoneWidthPct(z Zone) float64 {
	mid := z.Mid()
	if mid <= 0 {
		return 0
	}
	return (z.High - z.Low) / mid
}

func CapZoneWidth(z Zone, maxHighOverLow float64) Zone {
	if !z.Valid() || maxHighOverLow <= 1 {
		return z
	}
	maxHigh := z.Low * maxHighOverLow
	if z.High > maxHigh {
		z.High = maxHigh
	}
	return z
}
