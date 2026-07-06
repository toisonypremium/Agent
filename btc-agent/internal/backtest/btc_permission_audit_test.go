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

func TestBTCPermissionAuditScoreRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 140, 100)}
	got, err := RunBTCPermissionAudit(cfg, btc, BTCPermissionAuditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ScoreRows) != len(btcPermissionOrder()) {
		t.Fatalf("score rows=%d: %+v", len(got.ScoreRows), got.ScoreRows)
	}
	for i, perm := range btcPermissionOrder() {
		if got.ScoreRows[i].Permission != perm {
			t.Fatalf("score row %d permission=%s want %s", i, got.ScoreRows[i].Permission, perm)
		}
	}
}

func TestPermissionUnlockConditionsExplainWatchGap(t *testing.T) {
	analysis := agent1.MarketAnalysis{ActionPermission: agent1.Watch, TrendScore: 19.8, MarketRegime: "DOWNTREND", RiskLevel: agent1.Medium, FallingKnifeRisk: agent1.Medium, FomoRisk: agent1.Low}
	analysis.Flow.Bias = flow.BiasNeutral
	analysis.Flow.Score = 0
	analysis.PrimarySupportZone = market.Zone{Low: 90, High: 100}
	analysis.ResistanceZone = market.Zone{Low: 140, High: 150}
	got := PermissionUnlockConditions(analysis)
	joined := ""
	for _, item := range got {
		joined += item.Reason + " "
	}
	if !strings.Contains(joined, "trend score") || !strings.Contains(joined, "flow") {
		t.Fatalf("unlock conditions missing trend/flow: %+v", got)
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

func TestBTCPermissionAuditBlockersByPermission(t *testing.T) {
	counts := map[agent1.Permission]map[string]int{
		agent1.Armed: {BlockerTrendBelow60: 2, BlockerFlowNeutral: 1},
		agent1.Watch: {BlockerTrendBelow45: 3},
	}
	permissionCounts := map[agent1.Permission]int{agent1.Armed: 4, agent1.Watch: 6}
	rows := finalizeBTCPermissionBlockersByPermission(counts, permissionCounts)
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3: %+v", len(rows), rows)
	}
	if rows[0].Permission != agent1.Armed || rows[0].Blocker != BlockerTrendBelow60 || rows[0].RateWithinPermission != 0.5 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
}
