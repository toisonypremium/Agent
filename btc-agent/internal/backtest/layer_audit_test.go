package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func overridePlan() agent2.Plan {
	return agent2.Plan{Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, DiscountZone: market.Zone{Low: 100, High: 120, Name: "support"}, Invalidation: 98.5, Layers: []agent2.Layer{{Index: 1, Price: 120, Notional: 120, Quantity: 1}, {Index: 2, Price: 110, Notional: 110, Quantity: 1}, {Index: 3, Price: 100, Notional: 100, Quantity: 1}}}}}
}

func TestApplySimulationOverridesInvalidationBuffer(t *testing.T) {
	got := applySimulationOverrides(overridePlan(), SimulationOverrides{InvalidationBuffer: 0.05, TargetSymbols: map[string]bool{"ETHUSDT": true}})
	if got.Assets[0].Invalidation != 95 {
		t.Fatalf("invalidation=%v want 95", got.Assets[0].Invalidation)
	}
}

func TestApplySimulationOverridesLayerDepth(t *testing.T) {
	got := applySimulationOverrides(overridePlan(), SimulationOverrides{LayerDepthMultiplier: 1.5, TargetSymbols: map[string]bool{"ETHUSDT": true}})
	layers := got.Assets[0].Layers
	if layers[0].Price != 120 {
		t.Fatalf("layer1 should stay at support high, got %v", layers[0].Price)
	}
	if layers[1].Price != 105 || layers[2].Price != 90 {
		t.Fatalf("unexpected deeper layer prices: %+v", layers)
	}
	if layers[1].Quantity != layers[1].Notional/layers[1].Price {
		t.Fatalf("quantity not recalculated: %+v", layers[1])
	}
}

func TestRunLayerAuditProducesRows(t *testing.T) {
	cfg := simConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	btc := map[string][]market.Candle{"1d": simCandles("BTCUSDT", 90, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": simCandles("ETHUSDT", 90, 100)}
	got, err := RunLayerAudit(cfg, btc, assets, LayerAuditConfig{InvalidationBuffers: []float64{0.015}, LayerDepthMultipliers: []float64{1}, TargetSymbols: []string{"ETHUSDT"}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || len(got.Rows) != 1 || got.Rows[0].Verdict == "" {
		t.Fatalf("expected one audit row with verdict: %+v", got)
	}
}

func TestLayerAuditMarkdownSection(t *testing.T) {
	got, err := RunBTC(Config{MinWindow1D: 30, HorizonDays: []int{1, 3}}, btcCandles(70))
	if err != nil {
		t.Fatal(err)
	}
	got.LayerAudit = LayerAuditResult{Enabled: true, Summary: "layer audit test", Rows: []LayerAuditRow{{Symbol: "ETHUSDT", InvalidationBuffer: 0.015, LayerDepthMultiplier: 1, PlansCreated: 1, OrdersFilled: 1, Verdict: "WATCH"}}}
	md := Markdown(got)
	if !strings.Contains(md, "Agent 2 Invalidation/Layer Audit") {
		t.Fatalf("markdown missing layer audit section:\n%s", md)
	}
}
