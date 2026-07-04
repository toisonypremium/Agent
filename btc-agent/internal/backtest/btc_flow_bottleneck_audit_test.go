package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestRunBTCFlowBottleneckAuditProducesRows(t *testing.T) {
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	got, err := RunBTCFlowBottleneckAudit(btc, BTCFlowBottleneckAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" || len(got.BiasRows) == 0 || len(got.ParamRows) == 0 {
		t.Fatalf("expected enabled rows with summary: %+v", got)
	}
}

func TestBTCFlowComponentExtraction(t *testing.T) {
	params := flow.DefaultParams()
	sig := flow.Signal{SweepLow: true, ReclaimSupport: true, Absorption: true, FlowBias: flow.BiasAccumulation, BullScore: 0.55, Confidence: 0.55}
	got := btcFlowComponents(sig, params)
	for _, want := range []string{FlowComponentSweepLow, FlowComponentReclaimSupport, FlowComponentAbsorption} {
		if !hasString(got, want) {
			t.Fatalf("components=%v missing %s", got, want)
		}
	}
	if hasString(got, FlowComponentNeutral) || hasString(got, FlowComponentWeakScore) {
		t.Fatalf("unexpected neutral/weak components: %v", got)
	}
}

func TestBTCFlowBottleneckParamRowsStable(t *testing.T) {
	sets := btcFlowParamSets()
	if len(sets) == 0 || sets[0].name != "current" {
		t.Fatalf("first param set should be current: %+v", sets)
	}
	found := false
	for _, set := range sets {
		if set.name == "balanced_looser" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing balanced_looser: %+v", sets)
	}
}

func TestBTCFlowBottleneckSummaryMentionsNeutralAndWeak(t *testing.T) {
	components := []BTCFlowComponentAuditRow{
		{Component: FlowComponentNeutral, Count: 8, Rate: 0.8},
		{Component: FlowComponentWeakScore, Count: 7, Rate: 0.7},
	}
	biases := []BTCFlowBiasAuditRow{{Bias: flow.BiasNeutral, Count: 8, Rate: 0.8}}
	params := []BTCFlowParamAuditRow{{Name: "current", Verdict: BTCFlowParamVerdictBaseline}, {Name: "looser_accum_score", Verdict: BTCFlowParamVerdictCandidate}}
	got := summarizeBTCFlowBottleneckAudit(components, biases, params, 10)
	if !strings.Contains(got, "neutral=") || !strings.Contains(got, "weak_score=") || !strings.Contains(got, "best_param=looser_accum_score") {
		t.Fatalf("summary missing expected fields: %s", got)
	}
}
