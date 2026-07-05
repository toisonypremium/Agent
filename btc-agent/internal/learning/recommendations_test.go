package learning

import (
	"strings"
	"testing"

	"btc-agent/internal/backtest"
)

func TestBuildRecommendationsFlowCandidate(t *testing.T) {
	result := BuildRecommendations(backtest.Result{FlowParamQualityAudit: backtest.FlowParamQualityAuditResult{Enabled: true, Rows: []backtest.FlowParamQualityAuditRow{
		{Name: "current", BullishCount: 10, BullishAvgReturn: map[int]float64{7: 0.01}, FalsePositiveRate: 0.30, Score: 0.01, Verdict: backtest.FlowParamQualityKeepCurrent},
		{Name: "sensitive", BullishCount: 20, AddedBullishCount: 8, BullishAvgReturn: map[int]float64{7: 0.03}, FalsePositiveRate: 0.20, Score: 0.03, Verdict: backtest.FlowParamQualityCandidateTune},
	}}})
	if len(result.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	got := result.Recommendations[0]
	if got.Area != AreaFlowParams || got.Severity != SeverityActionable {
		t.Fatalf("unexpected top recommendation: %+v", got)
	}
	if !strings.Contains(got.ManualAction, "manual review") {
		t.Fatalf("manual action should require manual review: %s", got.ManualAction)
	}
}

func TestBuildRecommendationsDataQualityFallback(t *testing.T) {
	result := BuildRecommendations(backtest.Result{})
	if len(result.Recommendations) != 1 {
		t.Fatalf("recommendations=%d want 1", len(result.Recommendations))
	}
	got := result.Recommendations[0]
	if got.Area != AreaDataQuality || got.Confidence != ConfidenceLow {
		t.Fatalf("unexpected fallback: %+v", got)
	}
}

func TestBuildRecommendationsLayerAndExitCandidates(t *testing.T) {
	result := BuildRecommendations(backtest.Result{
		LayerAudit: backtest.LayerAuditResult{Enabled: true, Rows: []backtest.LayerAuditRow{{Symbol: "ETHUSDT", InvalidationBuffer: 0.03, LayerDepthMultiplier: 1.25, OrdersPlaced: 12, OrdersFilled: 8, FinalPnL: 20, MaxDrawdown: -0.05, Verdict: "CANDIDATE"}}},
		ExitAudit:  backtest.ExitAuditResult{Enabled: true, Rows: []backtest.ExitAuditRow{{Symbol: "ETHUSDT", TakeProfitPct: 0.05, TimeStopDays: 7, OrdersPlaced: 12, TakeProfits: 5, FinalPnL: 15, MaxDrawdown: -0.04, Verdict: "CANDIDATE"}}},
	})
	areas := map[string]bool{}
	for _, rec := range result.Recommendations {
		areas[rec.Area] = true
		text := strings.ToLower(rec.Recommendation + " " + rec.ManualAction)
		for _, unsafe := range []string{"place live", "submit order", "auto-tune", "auto tune", "override deterministic"} {
			if strings.Contains(text, unsafe) {
				t.Fatalf("recommendation contains unsafe phrase %q: %+v", unsafe, rec)
			}
		}
	}
	if !areas[AreaLayering] || !areas[AreaExit] {
		t.Fatalf("missing layer/exit recommendations: %+v", result.Recommendations)
	}
}
