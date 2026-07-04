package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

func TestRunBTCPermissionAuditProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	got, err := RunBTCPermissionAudit(cfg, btc, BTCPermissionAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Summary == "" {
		t.Fatalf("expected enabled result with summary: %+v", got)
	}
	want := btcPermissionOrder()
	if len(got.Rows) != len(want) {
		t.Fatalf("rows=%d want %d: %+v", len(got.Rows), len(want), got.Rows)
	}
	for i, perm := range want {
		if got.Rows[i].Permission != perm {
			t.Fatalf("row %d permission=%s want %s", i, got.Rows[i].Permission, perm)
		}
	}
}

func TestBTCPermissionAuditCountsBlockers(t *testing.T) {
	analysis := agent1.MarketAnalysis{
		ActionPermission: agent1.Watch,
		MarketRegime:     "DOWNTREND",
		RiskLevel:        agent1.Medium,
		FallingKnifeRisk: agent1.Medium,
		Flow: flow.MultiFrame{
			Bias:  flow.BiasNeutral,
			Score: 0.10,
		},
	}
	blockers := btcPermissionBlockers(analysis)
	for _, want := range []string{BlockerRegimeDowntrend, BlockerRiskMedium, BlockerFallingKnifeMedium, BlockerFlowNeutral, BlockerFlowWeakScore} {
		if !hasString(blockers, want) {
			t.Fatalf("blockers=%v missing %s", blockers, want)
		}
	}
}

func TestBTCPermissionAuditRowsStable(t *testing.T) {
	acc := map[agent1.Permission]*btcPermissionAcc{}
	rows := []BTCPermissionAuditRow{}
	for _, perm := range btcPermissionOrder() {
		rows = append(rows, finalizeBTCPermissionRow(perm, acc[perm], []int{3, 7}, 0))
	}
	want := btcPermissionOrder()
	for i, perm := range want {
		if rows[i].Permission != perm || rows[i].Count != 0 {
			t.Fatalf("row %d=%+v want permission=%s count=0", i, rows[i], perm)
		}
	}
}

func TestBTCPermissionAuditSummaryIncludesAllowedRate(t *testing.T) {
	rows := []BTCPermissionAuditRow{
		{Permission: agent1.Allowed, Count: 2, Rate: 0.2},
		{Permission: agent1.Armed, Count: 1, Rate: 0.1},
		{Permission: agent1.Watch, Count: 6, Rate: 0.6},
		{Permission: agent1.NoTrade, Count: 1, Rate: 0.1},
	}
	blockers := []BTCPermissionBlockerRow{{Blocker: BlockerTrendBelow60, Count: 5, Rate: 0.5}}
	got := summarizeBTCPermissionAudit(rows, blockers, 10)
	if !strings.Contains(got, "allowed=") || !strings.Contains(got, "top_blocker=") {
		t.Fatalf("summary missing expected fields: %s", got)
	}
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
