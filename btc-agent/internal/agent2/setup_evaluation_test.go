package agent2

import (
	"testing"

	"btc-agent/internal/market"
)

func TestSetupEvaluationAccumulatesSoftMisses(t *testing.T) {
	cfg := testConfig()
	cfg.Risk.DisableRelativeStrengthFilter = true
	cfg.Risk.MinRewardRisk = 3
	cfg.Risk.MinRotationScore = 0.8
	cfg.Risk.MaxRotationRank = 2
	asset := assetCandles(80, false)
	rotation := AssetRotationScore{Symbol: "ETHUSDT", Rank: 3, Score: 0.4, Eligible: true, Reason: "weak rotation"}
	ap, eval := evaluateAssetSetup(cfg, "ETHUSDT", asset, assetCandles(80, false), rotation, true)
	if len(eval.HardBlockers) != 0 {
		t.Fatalf("expected no hard blockers: %+v", eval)
	}
	for _, name := range []string{EntryCheckRotationScore, EntryCheckRotationRank, EntryCheckAssetFlowEntry, EntryCheckMMAccumulation} {
		if !setupGateFailed(eval.Gates, name) {
			t.Fatalf("expected failed setup gate %s: %+v", name, eval.Gates)
		}
	}
	if len(ap.SoftBlockers) < 3 || ap.State != StateWatch || len(ap.SetupGates) == 0 || ap.SetupScore <= 0 {
		t.Fatalf("expected accumulated soft blockers and positive setup metric: ap=%+v eval=%+v", ap, eval)
	}
}

func TestSetupEvaluationSoftLowerLowsDoNotHardBlock(t *testing.T) {
	cfg := testConfig()
	candles := assetCandles(80, false)
	for i := 76; i < 80; i++ {
		candles[i].Low = candles[i-1].Low - 5
		candles[i].Close = candles[i].Low + 1
	}
	ap := planAsset(cfg, "ETHUSDT", candles, nil, AssetRotationScore{}, false)
	if ap.State == StateNoTrade || len(ap.HardBlockers) != 0 || len(ap.Layers) != 0 {
		t.Fatalf("mild lower lows should soft wait without layers: %+v", ap)
	}
	if !setupGateFailed(ap.SetupGates, EntryCheckFallingKnife) {
		t.Fatalf("expected falling knife soft gate fail: %+v", ap.SetupGates)
	}
}

func TestSetupEvaluationHardDangerBlocks(t *testing.T) {
	cfg := testConfig()
	candles := assetCandles(80, false)
	last := len(candles) - 1
	candles[last].Close = candles[last-1].Low - 5
	candles[last].Low = candles[last].Close - 2
	candles[last].High = candles[last].Close + 1
	candles[last].Volume = candles[last-1].Volume * 2
	ap := planAsset(cfg, "ETHUSDT", candles, nil, AssetRotationScore{}, false)
	if ap.State != StateNoTrade || len(ap.HardBlockers) == 0 || len(ap.Layers) != 0 {
		t.Fatalf("hard danger should block without layers: %+v", ap)
	}
}

func TestWatchlistChecklistCanUseSetupGates(t *testing.T) {
	cfg := testConfig()
	plan := AssetPlan{
		Symbol:     "ETHUSDT",
		State:      StateWatch,
		SetupScore: 0.8,
		SetupGates: []SetupGateResult{
			{Name: EntryCheckFallingKnife, Pass: true, Severity: SetupGateHard, Score: 1},
			{Name: EntryCheckFOMO, Pass: true, Severity: SetupGateHard, Score: 1},
			{Name: EntryCheckAssetFlowEntry, Pass: false, Severity: SetupGateSoft, Score: 0.5, Reason: "asset flow entry chưa xác nhận"},
		},
		DiscountZone:     market.Zone{Name: "support", Low: 90, High: 100},
		RewardRiskDetail: RewardRiskResult{Entry: 95, Invalidation: 88, Target: 130, Valid: true, Ratio: 5},
	}
	assets := map[string][]market.Candle{"ETHUSDT": assetCandles(80, true)}
	got := BuildWatchlist(cfg, assets, assetCandles(80, false), nil, []AssetPlan{plan})
	if len(got.Candidates) == 0 {
		t.Fatalf("expected candidate: %+v", got)
	}
	item, ok := checklistItem(got.Candidates[0].EntryChecklist, EntryCheckAssetFlowEntry)
	if !ok || item.Pass || item.Severity != EntryCheckSoft {
		t.Fatalf("expected setup gate checklist soft wait: %+v", got.Candidates[0].EntryChecklist)
	}
}

func setupGateFailed(gates []SetupGateResult, name string) bool {
	for _, gate := range gates {
		if gate.Name == name && !gate.Pass {
			return true
		}
	}
	return false
}
