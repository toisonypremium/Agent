package market

import "time"

type Candle struct {
	Symbol    string    `json:"symbol"`
	Interval  string    `json:"interval"`
	OpenTime  time.Time `json:"open_time"`
	CloseTime time.Time `json:"close_time"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

type Zone struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
	Name string  `json:"name,omitempty"`
}

func (z Zone) Valid() bool { return z.Low > 0 && z.High >= z.Low }
func (z Zone) Mid() float64 {
	if !z.Valid() {
		return 0
	}
	return (z.Low + z.High) / 2
}

func LastClose(cs []Candle) float64 {
	if len(cs) == 0 {
		return 0
	}
	return cs[len(cs)-1].Close
}
