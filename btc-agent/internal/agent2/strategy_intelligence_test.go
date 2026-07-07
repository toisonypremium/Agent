package agent2

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/liquidity"
)

func TestBuildStrategyIntelligenceReportsSpecificGaps(t *testing.T) {
	cfg := testConfig()
	asset := AssetPlan{
		Symbol:           "ETHUSDT",
		State:            StateWatch,
		SetupScore:       0.62,
		RewardRisk:       2.2,
		DiscountGapPct:   0.12,
		RotationRank:     3,
		RotationScore:    0.41,
		AssetFlowScore:   0.36,
		MMScore:          28,
		MMMissing:        []string{"chưa reclaim"},
		LiquidityQuality: liquidity.Quality{Enabled: true, Pass: false, Score: 52, Grade: liquidity.GradeD, Reasons: []string{"thin proxy"}},
		SetupGates: []SetupGateResult{
			{Name: EntryCheckRewardRisk, Severity: SetupGateSoft, Pass: false, Score: 0.85, Reason: "rr low", Next: "wait deeper"},
			{Name: EntryCheckDiscountZone, Severity: SetupGateSoft, Pass: false, Score: 0.70, Reason: "price high", Next: "wait support"},
			{Name: EntryCheckAssetFlowEntry, Severity: SetupGateSoft, Pass: false, Score: 0.50, Reason: "flow weak", Next: "wait reclaim"},
		},
	}
	got := BuildStrategyIntelligence(cfg, asset)
	if !got.ResearchOnly {
		t.Fatal("strategy intelligence must be research-only")
	}
	if got.ClosestGate != EntryCheckRewardRisk {
		t.Fatalf("closest gate=%s want %s", got.ClosestGate, EntryCheckRewardRisk)
	}
	joined := got.Summary
	for _, gap := range got.UnlockGaps {
		joined += " " + gap.Reason
	}
	for _, want := range []string{"RR_gap=0.80", "discount_gap=12.0%", "flow/MM=0.36/28.0", "missing chưa reclaim"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestBuildResearchSizingSuggestionCappedAndNonActiveNoAuthority(t *testing.T) {
	cfg := testConfig()
	cfg.Live.Enabled = true
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 5
	cfg.Live.MaxOrderNotionalUSDT = 10
	asset := AssetPlan{Symbol: "ETHUSDT", State: StateScout, SetupScore: 0.80, LiquidityQuality: liquidity.Quality{Enabled: true, Score: 75, Grade: liquidity.GradeB, Pass: true}}
	got := BuildResearchSizingSuggestion(cfg, asset, agent1.Armed)
	if !got.ResearchOnly || !got.OrderAuthorityUnchanged {
		t.Fatalf("sizing must be research-only with unchanged authority: %+v", got)
	}
	if got.CapUSDT > 5 {
		t.Fatalf("cap must respect live canary context when live enabled: %+v", got)
	}
	if got.SuggestedNotionalUSDT > got.CapUSDT {
		t.Fatalf("suggestion exceeds cap: %+v", got)
	}
	if got.ExecutableNowUSDT != 0 || !strings.Contains(got.Reason, "no order authority") {
		t.Fatalf("non-active state must not be executable: %+v", got)
	}
}

func TestBuildResearchSizingSuggestionOnlyActiveAllowedExecutable(t *testing.T) {
	cfg := testConfig()
	asset := AssetPlan{Symbol: "ETHUSDT", State: StateActiveLimit, SetupScore: 0.90, LiquidityQuality: liquidity.Quality{Enabled: true, Score: 100, Grade: liquidity.GradeA, Pass: true}}
	got := BuildResearchSizingSuggestion(cfg, asset, agent1.Allowed)
	if got.ExecutableNowUSDT <= 0 {
		t.Fatalf("active+allowed can be executable in research suggestion: %+v", got)
	}
	if got.SuggestedNotionalUSDT > got.CapUSDT {
		t.Fatalf("suggestion exceeds cap: %+v", got)
	}
	for _, state := range []State{StateWatch, StateScout, StateArmed} {
		asset.State = state
		got = BuildResearchSizingSuggestion(cfg, asset, agent1.Allowed)
		if got.ExecutableNowUSDT != 0 {
			t.Fatalf("%s must not be executable: %+v", state, got)
		}
	}
}
