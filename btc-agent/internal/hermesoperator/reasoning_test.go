package hermesoperator

import (
	"reflect"
	"testing"
)

func strongInput() QuantReasoningInput {
	return QuantReasoningInput{Symbol: "RENDERUSDT", ProbeEligible: true, SetupScore: .74, MMScore: 25, FlowScore: 0, RotationScore: .31, LiquidityScore: 100, RewardRisk: 19.7, EntryPrice: 1.48, Invalidation: 1.42, Target: 2.43, MarketRegime: "DOWNTREND", AccumulationPhase: "SELL_ABSORPTION", DataQuality: .9, TotalCapital: 3275, MaxProbeCapitalPct: .05, MaxProbeNotional: 420}
}
func TestQuantReasoningDeterministic(t *testing.T) {
	a := ComputeQuantReasoning(strongInput())
	b := ComputeQuantReasoning(strongInput())
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("nondeterministic: %+v %+v", a, b)
	}
}
func TestQuantReasoningSoftWeaknessCanRecommendTinyProbe(t *testing.T) {
	r := ComputeQuantReasoning(strongInput())
	if !r.Eligible || r.Recommendation != IntentProbeLimit {
		t.Fatalf("expected probe: %+v", r)
	}
	if r.SuggestedCapitalPct <= 0 || r.SuggestedCapitalPct > .05 || r.SuggestedNotionalUSDT > 420 {
		t.Fatalf("unsafe sizing: %+v", r)
	}
	if r.ExpectedR <= 0 {
		t.Fatalf("expected positive utility: %+v", r)
	}
}
func TestQuantReasoningRejectsUnsafeEnvelope(t *testing.T) {
	in := strongInput()
	in.ProbeEligible = false
	r := ComputeQuantReasoning(in)
	if r.Eligible || r.Recommendation == IntentProbeLimit {
		t.Fatalf("unsafe probe: %+v", r)
	}
	in = strongInput()
	in.Invalidation = in.EntryPrice
	r = ComputeQuantReasoning(in)
	if r.Eligible {
		t.Fatalf("invalid downside accepted: %+v", r)
	}
}
func TestQuantReasoningHistoryIsShrunk(t *testing.T) {
	in := strongInput()
	base := ComputeQuantReasoning(in)
	in.HistoricalTrades = 1
	in.HistoricalWins = 1
	one := ComputeQuantReasoning(in)
	if one.PosteriorWinProbability >= .8 || one.PosteriorWinProbability <= base.PosteriorWinProbability {
		t.Fatalf("single win not properly shrunk: base=%+v one=%+v", base, one)
	}
	in.HistoricalTrades = 20
	in.HistoricalWins = 4
	in.HistoricalExpectancy = -.08
	bad := ComputeQuantReasoning(in)
	if bad.PosteriorWinProbability >= base.PosteriorWinProbability {
		t.Fatalf("bad history not penalized: base=%+v bad=%+v", base, bad)
	}
}
func TestQuantReasoningPanicSizesZero(t *testing.T) {
	in := strongInput()
	in.MarketRegime = "PANIC_SELLING"
	r := ComputeQuantReasoning(in)
	if r.Eligible || r.SuggestedNotionalUSDT != 0 {
		t.Fatalf("panic exposure allowed: %+v", r)
	}
}
