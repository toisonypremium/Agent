package backtest

import (
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/market"
	"btc-agent/internal/researchprofile"
)

func TestRunThresholdCalibrationProfilesStable(t *testing.T) {
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	btc := map[string][]market.Candle{"1d": calibrationCandles(120)}
	got, err := RunThresholdCalibration(cfg, btc, ThresholdCalibrationConfig{MinWindow1D: 60, HorizonDays: []int{7}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"STRICT_CURRENT", "BALANCED_SAFE", "ARMED_PROBE_LIGHT", "FLOW_RELAXED", "RR_RELAXED_SMALL_PROBE"}
	if len(got.Rows) != len(want) {
		t.Fatalf("rows=%d want %d", len(got.Rows), len(want))
	}
	for i, name := range want {
		if got.Rows[i].Profile.Name != name {
			t.Fatalf("row %d name=%s want %s", i, got.Rows[i].Profile.Name, name)
		}
	}
	if !got.Enabled || got.Summary == "" {
		t.Fatalf("bad result: %+v", got)
	}
}

func TestThresholdStrictMatchesProductionPermission(t *testing.T) {
	cfg := config.Config{}
	candles := calibrationCandles(90)
	analysis, err := agent1.Analyze(cfg, map[string][]market.Candle{"1d": candles, "4h": candles, "1w": candles}, exchange.FearGreed{Value: 50})
	if err != nil {
		t.Fatal(err)
	}
	strict := researchprofile.Profiles()[0]
	if got := researchprofile.EvaluatePermission(analysis, strict); got != analysis.ActionPermission {
		t.Fatalf("strict permission=%s production=%s analysis=%+v", got, analysis.ActionPermission, analysis)
	}
}

func TestThresholdCalibrationMarkdown(t *testing.T) {
	r := Result{ThresholdCalibration: ThresholdCalibrationResult{Enabled: true, Summary: "ok", Rows: []ThresholdProfileRow{{Profile: ThresholdProfile{Name: "STRICT_CURRENT"}, Windows: 10, ArmedRate: 0.1, AllowedRate: 0.2, AvgReturn: map[int]float64{7: 0.01}, WinRate: map[int]float64{7: 0.6}, WorstDrawdown: map[int]float64{7: -0.03}, Verdict: ThresholdKeepCurrent}}}}
	md := Markdown(r)
	for _, want := range []string{"Threshold Calibration Profiles", "STRICT_CURRENT", "Research-only: no production threshold changed"} {
		if !containsText(md, want) {
			t.Fatalf("missing %q in markdown", want)
		}
	}
}

func calibrationCandles(n int) []market.Candle {
	out := make([]market.Candle, n)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range out {
		px := 100.0 + float64(i)*0.5
		out[i] = market.Candle{Symbol: "BTCUSDT", Interval: "1d", OpenTime: now.Add(time.Duration(i) * 24 * time.Hour), CloseTime: now.Add(time.Duration(i+1) * 24 * time.Hour), Open: px, High: px * 1.02, Low: px * 0.98, Close: px * 1.01, Volume: 1000 + float64(i)}
	}
	return out
}

func containsText(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestThresholdCalibrationUsesPurgedChronologicalSplits(t *testing.T) {
	cfg := config.Config{}
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	got, err := RunThresholdCalibration(cfg, map[string][]market.Candle{"1d": calibrationCandles(240)}, ThresholdCalibrationConfig{MinWindow1D: 60, HorizonDays: []int{7}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Split.CalibrationStart-got.Split.ProfileEnd < 7 || got.Split.ValidationStart-got.Split.CalibrationEnd < 7 {
		t.Fatalf("embargo missing: %+v", got.Split)
	}
	if got.Split.ProfileEnd >= got.Split.CalibrationStart || got.Split.CalibrationEnd >= got.Split.ValidationStart {
		t.Fatalf("non chronological: %+v", got.Split)
	}
	if len(got.ValidationRows) == 0 || got.Rows[0].Profile.Name != got.ValidationRows[0].Profile.Name {
		t.Fatalf("final rows must be validation only")
	}
}
