package backtest

import "btc-agent/internal/market"

const (
	OHLCVExitNone         = "NONE"
	OHLCVExitInvalidation = "INVALIDATION"
	OHLCVExitTakeProfit   = "TAKE_PROFIT"
	OHLCVExitTimeStop     = "TIME_STOP"
)

type OHLCVAmbiguityDecision struct {
	Exit            string `json:"exit"`
	Ambiguous       bool   `json:"ambiguous"`
	Conservative    bool   `json:"conservative"`
	Reason          string `json:"reason"`
	InvalidationHit bool   `json:"invalidation_hit"`
	TakeProfitHit   bool   `json:"take_profit_hit"`
}

func ConservativeOHLCVExitDecision(candle market.Candle, avgPrice, invalidation, takeProfit float64) OHLCVAmbiguityDecision {
	invHit := invalidation > 0 && candle.Low <= invalidation
	tpHit := takeProfit > 0 && candle.High >= takeProfit
	if invHit && tpHit {
		return OHLCVAmbiguityDecision{Exit: OHLCVExitInvalidation, Ambiguous: true, Conservative: true, InvalidationHit: true, TakeProfitHit: true, Reason: "same-candle TP/invalidation ambiguity; conservative invalidation-first assumption"}
	}
	if invHit {
		return OHLCVAmbiguityDecision{Exit: OHLCVExitInvalidation, InvalidationHit: true, Reason: "candle low crossed invalidation"}
	}
	if tpHit {
		return OHLCVAmbiguityDecision{Exit: OHLCVExitTakeProfit, TakeProfitHit: true, Reason: "candle high crossed take-profit"}
	}
	return OHLCVAmbiguityDecision{Exit: OHLCVExitNone, Reason: "no exit crossed"}
}
