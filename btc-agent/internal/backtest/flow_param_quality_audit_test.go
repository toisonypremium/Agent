package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/market"
)

func TestRunFlowParamQualityAuditProducesRows(t *testing.T) {
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 160, 100)}
	got, err := RunFlowParamQualityAudit(btc, FlowParamQualityAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" {
		t.Fatalf("expected enabled result with summary: %+v", got)
	}
	if len(got.Rows) != len(btcFlowParamSets()) {
		t.Fatalf("rows=%d want %d", len(got.Rows), len(btcFlowParamSets()))
	}
}

func TestFlowParamQualityVerdictRejectsNoisy(t *testing.T) {
	cfg := FlowParamQualityAuditConfig{PrimaryHorizonDay: 7, MinSignals: 8}
	current := FlowParamQualityAuditRow{Name: "current", BullishCount: 10, BullishAvgReturn: map[int]float64{7: 0.02}, BullishWorstDrawdown: map[int]float64{7: -0.03}}
	row := FlowParamQualityAuditRow{Name: "candidate", BullishCount: 12, FalsePositiveRate: 0.70, DeepDrawdownRate: 0.10, BullishAvgReturn: map[int]float64{7: 0.03}, BullishWorstDrawdown: map[int]float64{7: -0.03}}
	if got := flowParamQualityVerdict(row, current, cfg); got != FlowParamQualityRejectNoisy {
		t.Fatalf("verdict=%s want %s", got, FlowParamQualityRejectNoisy)
	}
}

func TestFlowParamQualityVerdictCandidateTune(t *testing.T) {
	cfg := FlowParamQualityAuditConfig{PrimaryHorizonDay: 7, MinSignals: 8}
	current := FlowParamQualityAuditRow{Name: "current", BullishCount: 10, FalsePositiveRate: 0.30, BullishAvgReturn: map[int]float64{7: 0.02}, BullishWorstDrawdown: map[int]float64{7: -0.04}}
	row := FlowParamQualityAuditRow{Name: "candidate", BullishCount: 12, AddedBullishCount: 3, FalsePositiveRate: 0.35, DeepDrawdownRate: 0.10, BullishAvgReturn: map[int]float64{7: 0.025}, BullishWorstDrawdown: map[int]float64{7: -0.05}}
	if got := flowParamQualityVerdict(row, current, cfg); got != FlowParamQualityCandidateTune {
		t.Fatalf("verdict=%s want %s", got, FlowParamQualityCandidateTune)
	}
}

func TestFlowParamQualitySummaryMentionsBest(t *testing.T) {
	rows := []FlowParamQualityAuditRow{
		{Name: "current", Verdict: FlowParamQualityKeepCurrent, BullishCount: 10, Score: 0.01},
		{Name: "balanced_looser", Verdict: FlowParamQualityCandidateTune, BullishCount: 14, AddedBullishCount: 4, FalsePositiveRate: 0.25, Score: 0.02},
	}
	got := summarizeFlowParamQualityAudit(rows)
	if !strings.Contains(got, "best=") || !strings.Contains(got, "verdict=") {
		t.Fatalf("summary missing expected fields: %s", got)
	}
}
