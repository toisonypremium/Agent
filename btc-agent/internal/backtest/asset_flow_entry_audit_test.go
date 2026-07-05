package backtest

import (
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestRunAssetFlowEntryAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	assets := map[string][]market.Candle{"ETHUSDT": auditCandles("ETHUSDT", 140, 80), "SOLUSDT": auditCandles("SOLUSDT", 140, 60)}
	got, err := RunAssetFlowEntryAudit(cfg, assets, AssetFlowEntryAuditConfig{MinWindow1D: 30, HorizonDays: []int{3, 7}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || len(got.Rows) == 0 || got.Summary == "" {
		t.Fatalf("expected enabled rows with summary: %+v", got)
	}
}

func TestAssetFlowEntryTriggerClassification(t *testing.T) {
	if got := assetFlowEntryTrigger(agent2.AssetFlowEntrySignal{Pass: true}); got != AssetFlowEntryTriggerPass {
		t.Fatalf("trigger=%s want %s", got, AssetFlowEntryTriggerPass)
	}
	if got := assetFlowEntryTrigger(agent2.AssetFlowEntrySignal{HardBlock: true}); got != AssetFlowEntryTriggerHardBlock {
		t.Fatalf("trigger=%s want %s", got, AssetFlowEntryTriggerHardBlock)
	}
	if got := assetFlowEntryTrigger(agent2.AssetFlowEntrySignal{}); got != AssetFlowEntryTriggerSoftFail {
		t.Fatalf("trigger=%s want %s", got, AssetFlowEntryTriggerSoftFail)
	}
}

func TestAssetFlowEntryBucketAndVerdict(t *testing.T) {
	if got := assetFlowBullScoreBucket(0.24); got != "<0.25" {
		t.Fatalf("bucket=%s", got)
	}
	if got := assetFlowBullScoreBucket(0.25); got != "0.25-0.50" {
		t.Fatalf("bucket=%s", got)
	}
	if got := assetFlowBullScoreBucket(0.50); got != "0.50+" {
		t.Fatalf("bucket=%s", got)
	}
	row := AssetFlowEntryAuditRow{Symbol: "ETHUSDT", FlowBias: flow.BiasAccumulation, Trigger: AssetFlowEntryTriggerPass, Count: 5, AvgReturn: map[int]float64{7: 0.04}, WinRate: map[int]float64{7: 0.60}, WorstDrawdown: map[int]float64{7: -0.05}}
	if got := assetFlowEntryAuditVerdict(row, []int{7}); got != AssetFlowEntryVerdictCandidate {
		t.Fatalf("verdict=%s want %s", got, AssetFlowEntryVerdictCandidate)
	}
	row.Count = 4
	if got := assetFlowEntryAuditVerdict(row, []int{7}); got != AssetFlowEntryVerdictLowSample {
		t.Fatalf("verdict=%s want %s", got, AssetFlowEntryVerdictLowSample)
	}
}
