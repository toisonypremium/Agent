package backtest

import (
	"encoding/csv"
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func TestBuildTrainingDatasetProducesRows(t *testing.T) {
	cfg := triggerAuditConfig()
	btc := map[string][]market.Candle{"1d": auditCandles("BTCUSDT", 160, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": auditCandles("ETHUSDT", 160, 80), "SOLUSDT": auditCandles("SOLUSDT", 160, 60)}
	got, err := BuildTrainingDataset(cfg, btc, assets, t.TempDir(), TrainingDatasetConfig{MaxRows: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled || got.Rows == 0 || got.JSONLPath == "" || got.CSVPath == "" {
		t.Fatalf("expected dataset files with rows: %+v", got)
	}
}

func TestBTCDecisionLabelsRiskAvoided(t *testing.T) {
	got := btcDecisionLabel(agent1.NoTrade, 0.03, -0.10)
	if got != LabelRiskAvoided {
		t.Fatalf("label=%s want %s", got, LabelRiskAvoided)
	}
}

func TestAssetWatchlistLabelMissedEntry(t *testing.T) {
	candidate := agent2.WatchCandidate{Actionable: false, Tier: agent2.WatchTierEarly}
	got := assetWatchlistLabel(candidate, 0.10, -0.03)
	if got != LabelMissedEntry {
		t.Fatalf("label=%s want %s", got, LabelMissedEntry)
	}
}

func TestDecisionDatasetCSVEscapesFields(t *testing.T) {
	rows := []DecisionDatasetRow{{
		Timestamp:       "2026-01-01",
		RowType:         DecisionRowBTCPermission,
		Symbol:          "BTCUSDT",
		ForwardReturn:   map[int]float64{3: 0.01, 7: 0.02, 14: 0.03},
		ForwardDrawdown: map[int]float64{7: -0.04},
		Label:           LabelWatchCorrect,
		Explanation:     "contains, comma\nand newline",
	}}
	data, err := decisionDatasetCSV(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := csv.NewReader(strings.NewReader(string(data)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("csv parse failed: %v\n%s", err, string(data))
	}
	if len(records) != 2 || records[1][22] != rows[0].Explanation {
		t.Fatalf("csv did not preserve explanation: %#v", records)
	}
}
