package microstructure

import (
	"strings"
	"time"
)

const (
	DefaultTakerAnomalyZ = 1.5
	MinTakerAnomalyZ     = 1.0
	MaxTakerAnomalyZ     = 3.0
)

type FootprintObservation struct {
	ObservedAt time.Time `json:"observed_at"`
	Price      float64   `json:"price"`
	Score      float64   `json:"score"`
	Verdict    string    `json:"verdict"`
	ThresholdZ float64   `json:"threshold_z"`
}

type SymbolCalibration struct {
	TakerAnomalyZ     float64               `json:"taker_anomaly_z"`
	Pending           *FootprintObservation `json:"pending,omitempty"`
	Resolved          int                   `json:"resolved"`
	Successes         int                   `json:"successes"`
	BatchResolved     int                   `json:"batch_resolved"`
	BatchSuccesses    int                   `json:"batch_successes"`
	LastOutcomeReturn float64               `json:"last_outcome_return,omitempty"`
	LastUpdated       time.Time             `json:"last_updated,omitempty"`
}

type CalibrationState struct {
	Version int                           `json:"version"`
	Symbols map[string]*SymbolCalibration `json:"symbols"`
}

func NewCalibrationState() CalibrationState {
	return CalibrationState{Version: 1, Symbols: map[string]*SymbolCalibration{}}
}

func (s *CalibrationState) normalize(symbol string) *SymbolCalibration {
	if s.Version == 0 {
		s.Version = 1
	}
	if s.Symbols == nil {
		s.Symbols = map[string]*SymbolCalibration{}
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	c := s.Symbols[symbol]
	if c == nil {
		c = &SymbolCalibration{TakerAnomalyZ: DefaultTakerAnomalyZ}
		s.Symbols[symbol] = c
	}
	if c.TakerAnomalyZ < MinTakerAnomalyZ || c.TakerAnomalyZ > MaxTakerAnomalyZ {
		c.TakerAnomalyZ = DefaultTakerAnomalyZ
	}
	return c
}

func (s *CalibrationState) Thresholds() map[string]float64 {
	out := map[string]float64{}
	for symbol := range s.Symbols {
		out[strings.ToUpper(symbol)] = s.normalize(symbol).TakerAnomalyZ
	}
	return out
}

// ObserveAndLearn resolves one non-overlapping 24h observation per symbol.
// A +2% forward move is a success. Threshold changes are bounded to 0.05
// after each independent batch of eight resolved outcomes.
func (s *CalibrationState) ObserveAndLearn(now time.Time, signals map[string]MMFootprintSignal) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	for symbol, signal := range signals {
		c := s.normalize(symbol)
		if c.Pending != nil && now.Sub(c.Pending.ObservedAt) >= 24*time.Hour && signal.PriceLatest > 0 && c.Pending.Price > 0 {
			ret := (signal.PriceLatest - c.Pending.Price) / c.Pending.Price
			c.Resolved++
			c.BatchResolved++
			c.LastOutcomeReturn = ret
			if ret >= .02 {
				c.Successes++
				c.BatchSuccesses++
			}
			c.Pending = nil
			if c.BatchResolved >= 8 {
				precision := float64(c.BatchSuccesses) / float64(c.BatchResolved)
				if precision < .45 {
					c.TakerAnomalyZ = calClamp(c.TakerAnomalyZ+.05, MinTakerAnomalyZ, MaxTakerAnomalyZ)
				}
				if precision > .70 {
					c.TakerAnomalyZ = calClamp(c.TakerAnomalyZ-.05, MinTakerAnomalyZ, MaxTakerAnomalyZ)
				}
				c.BatchResolved = 0
				c.BatchSuccesses = 0
				c.LastUpdated = now
			}
		}
		if c.Pending == nil && signal.PriceLatest > 0 && (signal.Verdict == "POSSIBLE_ACCUMULATION" || signal.Verdict == "MM_ACCUMULATING") {
			c.Pending = &FootprintObservation{ObservedAt: now, Price: signal.PriceLatest, Score: signal.FootprintScore, Verdict: signal.Verdict, ThresholdZ: c.TakerAnomalyZ}
		}
	}
}

func calClamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
