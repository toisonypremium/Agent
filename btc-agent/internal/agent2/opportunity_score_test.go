package agent2

import "testing"

func TestBuildOpportunityCompositeStrongAsset(t *testing.T) {
	asset := AssetPlan{Symbol: "ETHUSDT", SetupScore: 0.95, RotationScore: 0.90, RewardRisk: 3.2, DiscountGapPct: -0.01, AssetFlowScore: 0.90, MMScore: 90}
	asset.LiquidityQuality.Score = 90
	got := BuildOpportunityComposite(asset)
	if got.Score < 80 || got.Verdict != OpportunityVerdictStrong {
		t.Fatalf("expected strong opportunity, got score=%.2f verdict=%s", got.Score, got.Verdict)
	}
}

func TestBuildOpportunityCompositeBlocksData(t *testing.T) {
	asset := AssetPlan{Symbol: "ETHUSDT", SetupScore: 0.2, SetupGates: []SetupGateResult{{Name: EntryCheckData, Pass: false, Severity: SetupGateHard, Reason: "chưa đủ dữ liệu 1D"}}}
	got := BuildOpportunityComposite(asset)
	if got.Verdict != OpportunityVerdictData || got.Score != 0 {
		t.Fatalf("expected data block score 0, got score=%.2f verdict=%s", got.Score, got.Verdict)
	}
}

func TestBuildOpportunityCompositeBlocksRisk(t *testing.T) {
	asset := AssetPlan{Symbol: "ETHUSDT", SetupScore: 0.8, SetupGates: []SetupGateResult{{Name: EntryCheckFallingKnife, Pass: false, Severity: SetupGateHard, Score: 0.1, Reason: "falling knife high"}}}
	got := BuildOpportunityComposite(asset)
	if got.Verdict != OpportunityVerdictRisk {
		t.Fatalf("expected risk block, got score=%.2f verdict=%s", got.Score, got.Verdict)
	}
}

func TestOpportunityCompositeFromComponentsClamps(t *testing.T) {
	got := OpportunityCompositeFromComponents("ETHUSDT", "", 0, map[string]float64{"setup_score": 3, "rotation_score": 3, "reward_risk_score": 3, "discount_score": 3, "asset_flow_score": 3, "mm_accumulation_score": 3, "liquidity_score": 3})
	if got.Score > 100 || got.Score < 0 {
		t.Fatalf("score not clamped: %.2f", got.Score)
	}
}
