package macroflow

import "testing"

func TestEvaluateDetectsSelectiveAltRotationWithoutGrantingAuthority(t *testing.T) {
	r := Evaluate(Input{
		BTCDominancePct: 56.56, BTCDominanceChange24hPctPoint: -0.11,
		USDTDominancePct: 7.90, USDTDominanceChange24hPctPoint: -0.20,
		BTCChange24hPct: 2.40, ETHChange24hPct: 3.88,
		BTCChange7dPct: 4.78, ETHChange7dPct: 8.16,
		BreadthSampleSize: 86, BreadthAdvancers24h: 69,
		MedianReturn24hPct: 1.84, AltWeightedReturn24hPct: 2.96,
		GlobalVolumeChange24hPct: 49.33,
		USDTSupplyChange30dPct:   -1.08,
		Fresh:                    true,
	})
	if r.Regime != RegimeEarlyAltRotation {
		t.Fatalf("regime=%s want %s", r.Regime, RegimeEarlyAltRotation)
	}
	if r.Action != ActionWaitPullback {
		t.Fatalf("action=%s want %s", r.Action, ActionWaitPullback)
	}
	if r.CanGrantExecution {
		t.Fatal("macro flow must never grant execution authority")
	}
	if !r.ETHLeading || r.BreadthRatio < 0.80 {
		t.Fatalf("unexpected leadership/breadth: %+v", r)
	}
}

func TestEvaluateFailsClosedWhenStale(t *testing.T) {
	r := Evaluate(Input{Fresh: false, BreadthSampleSize: 86, BreadthAdvancers24h: 80})
	if r.Regime != RegimeUnknown || r.Action != ActionBlockNewExposure {
		t.Fatalf("stale result must block: %+v", r)
	}
	if r.CanGrantExecution {
		t.Fatal("stale macro flow granted execution")
	}
}

func TestEvaluateDoesNotCallOneGreenDayConfirmedAltseason(t *testing.T) {
	r := Evaluate(Input{BTCDominanceChange24hPctPoint: -0.3, USDTDominanceChange24hPctPoint: -0.3, BTCChange24hPct: 1, ETHChange24hPct: 5, BTCChange7dPct: 8, ETHChange7dPct: 6, BreadthSampleSize: 90, BreadthAdvancers24h: 80, MedianReturn24hPct: 3, GlobalVolumeChange24hPct: 60, USDTSupplyChange30dPct: -3, Fresh: true})
	if r.Regime == RegimeConfirmedAltseason {
		t.Fatalf("one-day breadth must not confirm altseason: %+v", r)
	}
}

func TestBuildInputDerivesDominanceBreadthAndSupplyChange(t *testing.T) {
	in, err := BuildInput(GlobalSnapshot{MarketCapUSD: 1000, MarketCapChange24hPct: 10, VolumeChange24hPct: 20, UpdatedAtUnix: 200000}, []AssetSnapshot{
		{Symbol: "btc", MarketCapUSD: 600, MarketCapChange24hPct: 5, Change24hPct: 2, Change7dPct: 4},
		{Symbol: "eth", MarketCapUSD: 200, MarketCapChange24hPct: 12, Change24hPct: 5, Change7dPct: 8},
		{Symbol: "usdt", MarketCapUSD: 100, MarketCapChange24hPct: 0, Stable: true},
		{Symbol: "aaa", MarketCapUSD: 50, Change24hPct: 8},
		{Symbol: "bbb", MarketCapUSD: 50, Change24hPct: -2},
	}, []SupplyPoint{{Unix: 1, USD: 80}, {Unix: 200000, USD: 100}}, 200100, 300)
	if err != nil {
		t.Fatal(err)
	}
	if in.BreadthSampleSize != 4 || in.BreadthAdvancers24h != 3 {
		t.Fatalf("breadth=%+v", in)
	}
	if in.BTCDominanceChange24hPctPoint >= 0 {
		t.Fatalf("BTC dominance should fall: %+v", in)
	}
	if in.USDTDominanceChange24hPctPoint >= 0 {
		t.Fatalf("USDT dominance should fall: %+v", in)
	}
	if in.USDTSupplyChange30dPct != 25 {
		t.Fatalf("supply change=%.2f", in.USDTSupplyChange30dPct)
	}
}

func TestExecutionPolicyOnlyBlocksFreshRiskOff(t *testing.T) {
	if !BlocksNewExposure(Result{Regime: RegimeRiskOff, Input: Input{Fresh: true}}) {
		t.Fatal("fresh risk-off must block")
	}
	for _, r := range []Result{
		{Regime: RegimeEarlyAltRotation, Input: Input{Fresh: true}},
		{Regime: RegimeSelectiveRiskOn, Input: Input{Fresh: true}},
		{Regime: RegimeUnknown, Input: Input{Fresh: false}},
	} {
		if BlocksNewExposure(r) {
			t.Fatalf("advisory or unavailable macro must not replace existing deterministic authority: %+v", r)
		}
	}
}

func TestEvaluateDistinguishesMissingStableSupplyFromContraction(t *testing.T) {
	base := Input{BTCDominancePct: 56, USDTDominancePct: 8, BTCDominanceChange24hPctPoint: -.1, USDTDominanceChange24hPctPoint: -.2, BTCChange24hPct: 2, ETHChange24hPct: 4, BTCChange7dPct: 4, ETHChange7dPct: 8, BreadthSampleSize: 80, BreadthAdvancers24h: 60, MedianReturn24hPct: 1, Fresh: true}
	missing := Evaluate(base)
	if missing.StableLiquidityKnown {
		t.Fatal("missing supply history reported as known")
	}
	base.StableSupplyKnown = true
	base.USDTSupplyChange30dPct = -1
	contracting := Evaluate(base)
	if !contracting.StableLiquidityKnown || contracting.StableLiquidityUp {
		t.Fatalf("contracting supply classified incorrectly: %+v", contracting)
	}
}

func TestBuildInputDerivesTotal2Total3Changes(t *testing.T) {
	in, err := BuildInput(GlobalSnapshot{MarketCapUSD: 1000, MarketCapChange24hPct: 10, VolumeChange24hPct: 20, UpdatedAtUnix: 200000}, []AssetSnapshot{
		{Symbol: "btc", MarketCapUSD: 600, MarketCapChange24hPct: 5, Change24hPct: 2, Change7dPct: 4},
		{Symbol: "eth", MarketCapUSD: 200, MarketCapChange24hPct: 20, Change24hPct: 5, Change7dPct: 8},
		{Symbol: "usdt", MarketCapUSD: 100, MarketCapChange24hPct: 0, Stable: true},
		{Symbol: "aaa", MarketCapUSD: 100, MarketCapChange24hPct: 25, Change24hPct: 8},
	}, nil, 200100, 300)
	if err != nil {
		t.Fatal(err)
	}
	if in.Total2MarketCapUSD != 400 || in.Total3MarketCapUSD != 200 {
		t.Fatalf("TOTAL2/3=%+v", in)
	}
	if in.Total2Change24hPct <= 10 || in.Total3Change24hPct <= 10 {
		t.Fatalf("alts should outperform total market: %+v", in)
	}
}

func TestSupplyChangeSelectsNearestThirtyDayPoint(t *testing.T) {
	points := []SupplyPoint{{Unix: 0, USD: 80}, {Unix: 29 * 86400, USD: 100}, {Unix: 30 * 86400, USD: 110}, {Unix: 60 * 86400, USD: 121}}
	known, change := SupplyChange30d(points)
	if !known || change < 9.999999 || change > 10.000001 {
		t.Fatalf("known=%t change=%.8f", known, change)
	}
}
