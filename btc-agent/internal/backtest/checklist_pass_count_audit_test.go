package backtest

import (
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func TestRunChecklistPassCountAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": auditCandles("ETHUSDT", 140, 80), "SOLUSDT": auditCandles("SOLUSDT", 140, 60)}
	got, err := RunChecklistPassCountAudit(cfg, btc, assets, ChecklistPassCountAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" || len(got.Rows) == 0 {
		t.Fatalf("expected enabled rows with summary: %+v", got)
	}
}

func TestChecklistPassCountAuditCountsHardAndSoftFailures(t *testing.T) {
	a := newChecklistAuditAcc()
	candidate := agent2.WatchCandidate{EntryChecklist: []agent2.EntryChecklistItem{
		{Name: agent2.EntryCheckBTCPermission, Pass: false, Severity: agent2.EntryCheckHard},
		{Name: agent2.EntryCheckAssetFlowEntry, Pass: false, Severity: agent2.EntryCheckSoft},
		{Name: agent2.EntryCheckFOMO, Pass: true, Severity: agent2.EntryCheckHard},
	}}
	accumulateChecklistCandidate(a, candidate, 2)
	row := finalizeChecklistAuditRow("ETHUSDT", a)
	if row.HardFailRate != 1 || row.SoftFailRate != 1 {
		t.Fatalf("unexpected fail rates: %+v", row)
	}
	if row.HardFailCounts[agent2.EntryCheckBTCPermission] != 1 || row.SoftFailCounts[agent2.EntryCheckAssetFlowEntry] != 1 {
		t.Fatalf("unexpected fail counts: %+v", row)
	}
}

func TestChecklistPassCountAuditNearActionable(t *testing.T) {
	a := newChecklistAuditAcc()
	candidate := agent2.WatchCandidate{EntryChecklist: []agent2.EntryChecklistItem{
		{Name: agent2.EntryCheckBTCPermission, Pass: true, Severity: agent2.EntryCheckHard},
		{Name: agent2.EntryCheckAssetFlowEntry, Pass: false, Severity: agent2.EntryCheckSoft},
		{Name: agent2.EntryCheckDiscountZone, Pass: false, Severity: agent2.EntryCheckSoft},
	}}
	accumulateChecklistCandidate(a, candidate, 2)
	row := finalizeChecklistAuditRow("SOLUSDT", a)
	if row.NearActionableCount != 1 {
		t.Fatalf("expected near-actionable count: %+v", row)
	}
}

func TestChecklistPassCountAuditVerdictBlocked(t *testing.T) {
	row := ChecklistPassCountAuditRow{Samples: 10, HardFailRate: 0.8}
	if got := checklistPassCountVerdict(row); got != ChecklistVerdictBlocked {
		t.Fatalf("verdict=%s want %s", got, ChecklistVerdictBlocked)
	}
}
