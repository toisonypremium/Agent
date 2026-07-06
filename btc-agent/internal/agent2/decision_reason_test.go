package agent2

import "testing"

func TestDecisionReasonHelpers(t *testing.T) {
	reasons := []DecisionReason{}
	reasons = AddReason(reasons, NewDecisionReason(ReasonRotationScore, ReasonSoftWait, ReasonScopeRotation, "rotation wait"))
	reasons = AddReason(reasons, NewDecisionReason(ReasonFallingKnife, ReasonHardBlock, ReasonScopeRisk, "falling hard"))
	if !HasHardBlock(reasons) {
		t.Fatalf("expected hard block: %+v", reasons)
	}
	if !HasSoftWait(reasons) {
		t.Fatalf("expected soft wait: %+v", reasons)
	}
	codes := ReasonCodes(reasons)
	if len(codes) != 2 || codes[0] != string(ReasonRotationScore) || codes[1] != string(ReasonFallingKnife) {
		t.Fatalf("unexpected codes: %+v", codes)
	}
	if PrimaryReason(reasons) != "falling hard" {
		t.Fatalf("hard reason should be primary: %+v", reasons)
	}
	soft := ReasonsBySeverity(reasons, ReasonSoftWait)
	if len(soft) != 1 || soft[0].Code != ReasonRotationScore {
		t.Fatalf("unexpected soft reasons: %+v", soft)
	}
}

func TestPlanAssetLegacyStringsFromReasons(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.MinRotationScore = 0.8
	asset := assetCandles(80, false)
	ap := planAsset(cfg, "ETHUSDT", asset, assetCandles(80, false), AssetRotationScore{Symbol: "ETHUSDT", Rank: 3, Score: 0.4, Eligible: true, Reason: "weak rotation"}, true)
	if len(ap.Reasons) == 0 {
		t.Fatalf("expected typed reasons: %+v", ap)
	}
	if ap.Reason == "" {
		t.Fatalf("legacy reason should stay non-empty: %+v", ap)
	}
	if len(ap.SoftBlockers) == 0 && len(ap.HardBlockers) == 0 {
		t.Fatalf("legacy blockers should be populated from reasons: %+v", ap)
	}
}
