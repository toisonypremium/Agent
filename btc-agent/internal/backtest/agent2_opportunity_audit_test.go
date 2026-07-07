package backtest

import (
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/market"
)

func TestOpportunityAuditRowIdentifiesDominantBTCWait(t *testing.T) {
	var cfg config.Config
	cfg.Risk.MinRewardRisk = 3
	acc := &opportunityAcc{missing: map[string]int{}}
	asset := agent2.AssetPlan{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.65, RewardRisk: 2.5, DiscountGapPct: 0.04, SetupGates: []agent2.SetupGateResult{{Name: agent2.EntryCheckAssetFlowEntry, Pass: false}}, Reasons: []agent2.DecisionReason{{Code: agent2.ReasonBTCPermission, Severity: agent2.ReasonSoftWait, Scope: agent2.ReasonScopeBTC, Message: "BTC WATCH"}}}
	for i := 0; i < 10; i++ {
		accumulateOpportunity(acc, cfg, asset)
	}
	row := finalizeOpportunityRow("ETHUSDT", acc)
	if row.Samples != 10 || row.NearMissCount != 10 || row.BTCWaitRate != 1 || row.MissingGateCounts[agent2.EntryCheckBTCPermission] != 10 {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.ResearchOnlyVerdict != OpportunityVerdictWaitMarket {
		t.Fatalf("expected wait-market verdict: %+v", row)
	}
}

func TestOpportunityRecommendationTuneReviewForCloseDiscount(t *testing.T) {
	row := Agent2OpportunityAuditRow{Symbol: "ETHUSDT", Samples: 20, DiscountFailRate: 0.60, AvgDiscountGapPct: 0.08}
	_, verdict := opportunityRecommendation(row)
	if verdict != OpportunityVerdictTuneReview {
		t.Fatalf("expected tune review, got %s", verdict)
	}
}

func TestOpportunityAuditDoesNotMutateConfig(t *testing.T) {
	cfg := simConfig()
	before := cfg.Risk.MinRewardRisk
	_, _ = RunAgent2OpportunityAudit(cfg, map[string][]market.Candle{}, map[string][]market.Candle{}, Agent2OpportunityAuditConfig{})
	if cfg.Risk.MinRewardRisk != before {
		t.Fatalf("audit mutated config: before %.2f after %.2f", before, cfg.Risk.MinRewardRisk)
	}
}
