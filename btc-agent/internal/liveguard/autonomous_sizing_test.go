package liveguard

import (
	"btc-agent/internal/hermesoperator"
	"testing"
)

func TestAutonomousSizingWeakMarketReducesRatherThanBlocks(t *testing.T) {
	r := CalculateAutonomousSizing(AutonomousSizingContext{TotalCapital: 1000, Confidence: .8, Intent: hermesoperator.IntentProbeLimit, AccumulationPhase: "MARKDOWN", MarketRegime: "DOWNTREND", MMConfidence: .55, DataQuality: .9, LiquidityQuality: .8, RequestedNotional: 100, PerOrderCap: 100, AssetRemaining: 100, PortfolioRemaining: 100})
	if r.NotionalUSDT <= 0 || r.NotionalUSDT >= 100 {
		t.Fatalf("expected reduced non-zero size, got %+v", r)
	}
}
func TestAutonomousSizingConfidenceMonotonic(t *testing.T) {
	base := AutonomousSizingContext{TotalCapital: 1000, Intent: hermesoperator.IntentOpenLimit, AccumulationPhase: "RECLAIM", MarketRegime: "RANGE", DataQuality: 1, LiquidityQuality: 1, RequestedNotional: 1000, PerOrderCap: 1000, AssetRemaining: 1000, PortfolioRemaining: 1000}
	base.Confidence = .6
	low := CalculateAutonomousSizing(base)
	base.Confidence = .9
	high := CalculateAutonomousSizing(base)
	if high.NotionalUSDT <= low.NotionalUSDT {
		t.Fatalf("not monotonic: low=%+v high=%+v", low, high)
	}
}
func TestAutonomousSizingNeverExceedsCaps(t *testing.T) {
	r := CalculateAutonomousSizing(AutonomousSizingContext{TotalCapital: 100000, Confidence: 1, Intent: hermesoperator.IntentOpenLimit, AccumulationPhase: "ACCUMULATION_CONFIRMED", MarketRegime: "ACCUMULATION", DataQuality: 1, LiquidityQuality: 1, RequestedNotional: 9000, PerOrderCap: 100, AssetRemaining: 80, PortfolioRemaining: 70})
	if r.NotionalUSDT != 70 {
		t.Fatalf("expected tightest cap 70, got %+v", r)
	}
}
func TestAutonomousSizingPanicSeverelyReduced(t *testing.T) {
	r := CalculateAutonomousSizing(AutonomousSizingContext{TotalCapital: 1000, Confidence: 1, Intent: hermesoperator.IntentProbeLimit, AccumulationPhase: "RECLAIM", MarketRegime: "PANIC_SELLING", DataQuality: 1, LiquidityQuality: 1, RequestedNotional: 100})
	if r.NotionalUSDT <= 0 || r.NotionalUSDT >= 5 {
		t.Fatalf("panic must size a very small non-zero probe, got %+v", r)
	}
}
