package agent2

import "testing"

func TestRewardRisk(t *testing.T) {
	got := RewardRiskBreakdown(RewardRiskInput{Entry: 100, Invalidation: 90, Target: 130})
	if !got.Valid || got.Ratio != 3 {
		t.Fatalf("unexpected reward risk: %+v", got)
	}
}

func TestRewardRiskInvalid(t *testing.T) {
	got := RewardRiskBreakdown(RewardRiskInput{Entry: 90, Invalidation: 100, Target: 130})
	if got.Valid || got.Reason == "" {
		t.Fatalf("expected invalid rr with reason: %+v", got)
	}
}
