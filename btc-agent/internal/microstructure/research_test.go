package microstructure

import (
	"testing"

	"btc-agent/internal/market"
)

func TestEstimateLiquidationProxyIsReportOnlyAndFailClosed(t *testing.T) {
	blocked := EstimateLiquidationProxy(0, 100, 10, 9)
	if blocked.Status != "BLOCKED" || blocked.Direction != "UNKNOWN" {
		t.Fatalf("invalid observations must block: %+v", blocked)
	}
	proxy := EstimateLiquidationProxy(100, 98, 1000, 990)
	if proxy.Status != "PROXY" || proxy.Direction != "LONG_LIQUIDATION_PRESSURE" {
		t.Fatalf("expected long liquidation proxy: %+v", proxy)
	}
}

func TestAnchoredVWAPAndVolumeProfile(t *testing.T) {
	candles := []market.Candle{
		{Low: 9, High: 11, Close: 10, Volume: 2},
		{Low: 10, High: 14, Close: 12, Volume: 4},
	}
	vwap, err := AnchoredVWAP(candles, 0)
	want := float64(10*2+12*4) / 6
	if err != nil || vwap != want {
		t.Fatalf("unexpected anchored VWAP %.4f want %.4f: %v", vwap, want, err)
	}
	profile, err := BuildVolumeProfile(candles, 4)
	if err != nil || profile.Status != "OK" || profile.TotalVolume != 6 || len(profile.Levels) != 4 || profile.POC <= 0 {
		t.Fatalf("unexpected volume profile: %+v err=%v", profile, err)
	}
}

func TestVolumeProfileFlatRangeUsesSingleLevel(t *testing.T) {
	profile, err := BuildVolumeProfile([]market.Candle{{Low: 10, High: 10, Close: 10, Volume: 3}}, 12)
	if err != nil || len(profile.Levels) != 1 || profile.POC != 10 || profile.Levels[0].VolumeShare != 1 {
		t.Fatalf("unexpected flat profile: %+v err=%v", profile, err)
	}
}

func TestResearchIndicatorsDoNotRepresentExecutionAuthority(t *testing.T) {
	proxy := EstimateLiquidationProxy(100, 98, 1000, 990)
	if proxy.Status == "ALLOWED" || proxy.Status == "ACTIVE_LIMIT" {
		t.Fatal("research proxy must never encode execution authority")
	}
}
