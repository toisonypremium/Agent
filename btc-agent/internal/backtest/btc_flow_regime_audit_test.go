package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestRunBTCFlowRegimeAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	got, err := RunBTCFlowRegimeAudit(cfg, btc, BTCFlowRegimeAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" || len(got.Rows) == 0 {
		t.Fatalf("expected enabled rows with summary: %+v", got)
	}
	if !strings.Contains(got.Summary, "BTC flow by regime audit") {
		t.Fatalf("summary missing audit name: %s", got.Summary)
	}
}

func TestBTCFlowRegimeAuditVerdictLowSample(t *testing.T) {
	row := BTCFlowRegimeAuditRow{Count: 4, AvgReturn: map[int]float64{14: 0.20}, WinRate: map[int]float64{14: 1}, WorstDrawdown: map[int]float64{14: -0.01}}
	if got := btcFlowRegimeVerdict(row, []int{14}); got != BTCFlowRegimeVerdictLowSample {
		t.Fatalf("verdict=%s want %s", got, BTCFlowRegimeVerdictLowSample)
	}
}

func TestBTCFlowRegimeAuditSortStable(t *testing.T) {
	rows := []BTCFlowRegimeAuditRow{
		{Regime: "RANGE", Bias: flow.BiasNeutral, Count: 10, Verdict: BTCFlowRegimeVerdictReject},
		{Regime: "ACCUMULATION", Bias: flow.BiasAccumulation, Count: 6, Verdict: BTCFlowRegimeVerdictCandidate},
	}
	sortBTCFlowRegimeRows(rows)
	if rows[0].Verdict != BTCFlowRegimeVerdictCandidate {
		t.Fatalf("candidate should sort first: %+v", rows)
	}
}
