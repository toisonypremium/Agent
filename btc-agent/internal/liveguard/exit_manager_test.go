package liveguard

import (
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

// baseCfg returns a Config with ExitConfig fully populated for testing.
func testExitCfg(opts ...func(*config.ExitConfig)) config.Config {
	ec := config.ExitConfig{
		Enabled:               true,
		TakeProfitPct:         0.30,
		PartialExitPct:        0.50,
		TrailingActivatePct:   0.20,
		TrailingDistancePct:   0.08,
		TimeStopDays:          90,
		MinPnLForTimeStop:     0.0,
		PanicSellPnLThreshold: -0.25,
	}
	for _, o := range opts {
		o(&ec)
	}
	var cfg config.Config
	cfg.Exit = ec
	return cfg
}

func testPos(symbol string, qty, avgEntry float64, openedDaysAgo int) live.LivePosition {
	openedAt := time.Now().UTC().AddDate(0, 0, -openedDaysAgo).Unix()
	return live.LivePosition{
		Symbol:        symbol,
		Quantity:      qty,
		AvgEntryPrice: avgEntry,
		OpenedAt:      openedAt,
	}
}

// TestEvaluateExits_Disabled: cfg.Exit.Enabled=false → nil result.
func TestEvaluateExits_Disabled(t *testing.T) {
	cfg := testExitCfg(func(ec *config.ExitConfig) { ec.Enabled = false })
	positions := []live.LivePosition{testPos("ETHUSDT", 1.0, 2000.0, 10)}
	prices := map[string]float64{"ETHUSDT": 3000.0}
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result != nil {
		t.Fatalf("expected nil when disabled, got %v", result)
	}
}

// TestEvaluateExits_Hold: no condition triggers → HOLD.
func TestEvaluateExits_Hold(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("ETHUSDT", 1.0, 2000.0, 5)}
	// PnL = +5%, below take-profit (30%), no trail active, age < 90d
	prices := map[string]float64{"ETHUSDT": 2100.0}
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if len(result) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result))
	}
	if result[0].Action != ExitHold {
		t.Errorf("expected HOLD, got %s", result[0].Action)
	}
}

// TestEvaluateExits_TakeProfit: PnL >= take_profit_pct → TAKE_PROFIT with partial qty.
func TestEvaluateExits_TakeProfit(t *testing.T) {
	cfg := testExitCfg() // take_profit=30%, partial=50%
	positions := []live.LivePosition{testPos("SOLUSDT", 10.0, 100.0, 5)}
	prices := map[string]float64{"SOLUSDT": 135.0} // +35% > 30%
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if len(result) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result))
	}
	d := result[0]
	if d.Action != ExitTakeProfit {
		t.Errorf("expected TAKE_PROFIT, got %s: %s", d.Action, d.Reason)
	}
	wantQty := 10.0 * 0.50
	if d.SellQuantity != wantQty {
		t.Errorf("SellQuantity want %.4f got %.4f", wantQty, d.SellQuantity)
	}
}

// TestEvaluateExits_TakeProfit_FullExit: partial_exit_pct=0 → sell full position.
func TestEvaluateExits_TakeProfit_FullExit(t *testing.T) {
	cfg := testExitCfg(func(ec *config.ExitConfig) { ec.PartialExitPct = 0 })
	positions := []live.LivePosition{testPos("SOLUSDT", 10.0, 100.0, 5)}
	prices := map[string]float64{"SOLUSDT": 135.0}
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].SellQuantity != 10.0 {
		t.Errorf("expected full qty 10.0 got %.4f", result[0].SellQuantity)
	}
}

// TestEvaluateExits_TrailingStop_NotActive: trail not yet activated → HOLD.
func TestEvaluateExits_TrailingStop_NotActive(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("ETHUSDT", 1.0, 2000.0, 5)}
	// PnL = +10%, trail activates at 20% → not active
	prices := map[string]float64{"ETHUSDT": 2200.0}
	pt := NewPeakTracker()
	result := EvaluateExits(cfg, positions, prices, pt)
	if result[0].Action != ExitHold {
		t.Errorf("expected HOLD (trail not active), got %s", result[0].Action)
	}
	if pt.TrailActive["ETHUSDT"] {
		t.Error("trail should not be active at +10%")
	}
}

// TestEvaluateExits_TrailingStop_Triggered: trail active AND price fell from peak.
func TestEvaluateExits_TrailingStop_Triggered(t *testing.T) {
	cfg := testExitCfg() // trailing_activate=20%, trailing_distance=8%
	positions := []live.LivePosition{testPos("RENDERUSDT", 5.0, 1000.0, 10)}

	pt := NewPeakTracker()
	// Pre-seed: price rose to +25% (trail activates), peak = 1250
	pt.PeakBySymbol["RENDERUSDT"] = 1250.0
	pt.TrailActive["RENDERUSDT"] = true

	// trail_stop = 1250 * (1 - 0.08) = 1150; current 1140 <= 1150 → TRAILING_STOP
	prices := map[string]float64{"RENDERUSDT": 1140.0}
	result := EvaluateExits(cfg, positions, prices, pt)
	if len(result) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result))
	}
	d := result[0]
	if d.Action != ExitTrailingStop {
		t.Errorf("expected TRAILING_STOP, got %s: %s", d.Action, d.Reason)
	}
	if d.SellQuantity != 5.0 {
		t.Errorf("trailing stop should sell full qty 5.0, got %.4f", d.SellQuantity)
	}
}

// TestEvaluateExits_TrailingStop_AboveStop: trail active but price above trail stop → HOLD.
func TestEvaluateExits_TrailingStop_AboveStop(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("RENDERUSDT", 5.0, 1000.0, 10)}
	pt := NewPeakTracker()
	pt.PeakBySymbol["RENDERUSDT"] = 1250.0
	pt.TrailActive["RENDERUSDT"] = true
	// trail_stop = 1250 * 0.92 = 1150; current 1200 > 1150 → HOLD
	prices := map[string]float64{"RENDERUSDT": 1200.0}
	result := EvaluateExits(cfg, positions, prices, pt)
	if result[0].Action != ExitHold {
		t.Errorf("expected HOLD (above trail stop), got %s", result[0].Action)
	}
}

// TestEvaluateExits_TimeStop: position age > time_stop_days AND pnl >= min_pnl → TIME_STOP.
func TestEvaluateExits_TimeStop(t *testing.T) {
	cfg := testExitCfg() // time_stop_days=90, min_pnl=0.0
	positions := []live.LivePosition{testPos("ETHUSDT", 2.0, 2000.0, 95)} // 95d old
	prices := map[string]float64{"ETHUSDT": 2100.0}                        // +5% >= 0%
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action != ExitTimeStop {
		t.Errorf("expected TIME_STOP, got %s: %s", result[0].Action, result[0].Reason)
	}
	if result[0].SellQuantity != 2.0 {
		t.Errorf("time-stop should sell full qty 2.0, got %.4f", result[0].SellQuantity)
	}
}

// TestEvaluateExits_TimeStop_TooYoung: age < time_stop_days → HOLD.
func TestEvaluateExits_TimeStop_TooYoung(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("ETHUSDT", 2.0, 2000.0, 30)} // only 30d
	prices := map[string]float64{"ETHUSDT": 2100.0}
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action != ExitHold {
		t.Errorf("expected HOLD (too young), got %s", result[0].Action)
	}
}

// TestEvaluateExits_TimeStop_DeepLoss: pnl < min_pnl_for_time_stop → HOLD.
// MinPnLForTimeStop=0 means "no floor, always time-stop".
// To block time-stop in loss, operator must set MinPnLForTimeStop > 0 (e.g. 0.05 = require 5% gain).
func TestEvaluateExits_TimeStop_DeepLoss(t *testing.T) {
	cfg := testExitCfg(func(ec *config.ExitConfig) { ec.MinPnLForTimeStop = 0.05 }) // require >=5% gain
	positions := []live.LivePosition{testPos("ETHUSDT", 2.0, 2000.0, 95)}
	prices := map[string]float64{"ETHUSDT": 1800.0} // -10% < 5% floor
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action != ExitHold {
		t.Errorf("expected HOLD (pnl=-10%% < min_pnl_for_time_stop=5%%), got %s", result[0].Action)
	}
}

// TestEvaluateExits_TimeStop_ZeroFloor: MinPnLForTimeStop=0 → time-stop even in loss.
func TestEvaluateExits_TimeStop_ZeroFloor(t *testing.T) {
	cfg := testExitCfg(func(ec *config.ExitConfig) { ec.MinPnLForTimeStop = 0.0 }) // no floor
	positions := []live.LivePosition{testPos("ETHUSDT", 2.0, 2000.0, 95)}
	prices := map[string]float64{"ETHUSDT": 1800.0} // -10%, but no floor → time-stop anyway
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action != ExitTimeStop {
		t.Errorf("expected TIME_STOP (no floor, min_pnl=0), got %s", result[0].Action)
	}
}

// TestEvaluateExits_TimeStop_OpenedAtZero: OpenedAt=0 → time-stop must not trigger.
func TestEvaluateExits_TimeStop_OpenedAtZero(t *testing.T) {
	cfg := testExitCfg()
	p := live.LivePosition{Symbol: "ETHUSDT", Quantity: 1.0, AvgEntryPrice: 2000.0, OpenedAt: 0}
	prices := map[string]float64{"ETHUSDT": 2100.0}
	result := EvaluateExits(cfg, []live.LivePosition{p}, prices, NewPeakTracker())
	if result[0].Action == ExitTimeStop {
		t.Error("time-stop must not trigger when OpenedAt=0")
	}
}

// TestEvaluateExits_PanicSell: pnl <= panic threshold → PANIC_SELL full qty.
func TestEvaluateExits_PanicSell(t *testing.T) {
	cfg := testExitCfg() // panic_sell_pnl_threshold=-0.25
	positions := []live.LivePosition{testPos("SOLUSDT", 8.0, 100.0, 5)}
	prices := map[string]float64{"SOLUSDT": 72.0} // -28% <= -25%
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if len(result) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result))
	}
	d := result[0]
	if d.Action != ExitPanicSell {
		t.Errorf("expected PANIC_SELL, got %s: %s", d.Action, d.Reason)
	}
	if d.SellQuantity != 8.0 {
		t.Errorf("panic sell should sell full qty 8.0, got %.4f", d.SellQuantity)
	}
}

// TestEvaluateExits_PanicSell_Disabled: threshold=0 → never trigger.
func TestEvaluateExits_PanicSell_Disabled(t *testing.T) {
	cfg := testExitCfg(func(ec *config.ExitConfig) { ec.PanicSellPnLThreshold = 0 })
	positions := []live.LivePosition{testPos("SOLUSDT", 8.0, 100.0, 5)}
	prices := map[string]float64{"SOLUSDT": 50.0} // -50% — would trigger if enabled
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action == ExitPanicSell {
		t.Error("PANIC_SELL must not trigger when threshold=0")
	}
}

// TestEvaluateExits_PanicSell_JustAbove: pnl just above threshold → no panic.
func TestEvaluateExits_PanicSell_JustAbove(t *testing.T) {
	cfg := testExitCfg() // threshold=-0.25
	positions := []live.LivePosition{testPos("SOLUSDT", 5.0, 100.0, 5)}
	prices := map[string]float64{"SOLUSDT": 76.0} // -24% > -25%
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if result[0].Action == ExitPanicSell {
		t.Errorf("expected no panic (pnl=-24%% > threshold=-25%%), got %s", result[0].Action)
	}
}

// TestEvaluateExits_PanicSell_ConstValue: ExitPanicSell constant has expected string value.
func TestEvaluateExits_PanicSell_ConstValue(t *testing.T) {
	if ExitPanicSell != "PANIC_SELL" {
		t.Errorf("ExitPanicSell const wrong: %s", ExitPanicSell)
	}
}

// TestEvaluateExits_NoPriceForSymbol: missing price → skip position entirely.
func TestEvaluateExits_NoPriceForSymbol(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("ETHUSDT", 1.0, 2000.0, 5)}
	prices := map[string]float64{} // no price
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if len(result) != 0 {
		t.Errorf("expected 0 decisions (no price), got %d: %+v", len(result), result)
	}
}

// TestEvaluateExits_ZeroQty: zero quantity position → skip.
func TestEvaluateExits_ZeroQty(t *testing.T) {
	cfg := testExitCfg()
	p := live.LivePosition{Symbol: "ETHUSDT", Quantity: 0, AvgEntryPrice: 2000.0}
	prices := map[string]float64{"ETHUSDT": 3000.0}
	result := EvaluateExits(cfg, []live.LivePosition{p}, prices, NewPeakTracker())
	if len(result) != 0 {
		t.Errorf("expected 0 decisions (zero qty), got %d", len(result))
	}
}

// TestEvaluateExits_MultiPosition: each asset evaluated independently.
func TestEvaluateExits_MultiPosition(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{
		testPos("ETHUSDT", 1.0, 2000.0, 5),    // +5% → HOLD
		testPos("SOLUSDT", 10.0, 100.0, 5),    // +35% → TAKE_PROFIT
		testPos("RENDERUSDT", 3.0, 500.0, 95), // +2%, 95d old → TIME_STOP
	}
	prices := map[string]float64{
		"ETHUSDT":    2100.0,
		"SOLUSDT":    135.0,
		"RENDERUSDT": 510.0,
	}
	result := EvaluateExits(cfg, positions, prices, NewPeakTracker())
	if len(result) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(result))
	}
	bySymbol := map[string]ExitAction{}
	for _, d := range result {
		bySymbol[d.Symbol] = d.Action
	}
	if bySymbol["ETHUSDT"] != ExitHold {
		t.Errorf("ETHUSDT want HOLD got %s", bySymbol["ETHUSDT"])
	}
	if bySymbol["SOLUSDT"] != ExitTakeProfit {
		t.Errorf("SOLUSDT want TAKE_PROFIT got %s", bySymbol["SOLUSDT"])
	}
	if bySymbol["RENDERUSDT"] != ExitTimeStop {
		t.Errorf("RENDERUSDT want TIME_STOP got %s", bySymbol["RENDERUSDT"])
	}
}

// TestEvaluateExits_PeakTrackerPersists: peak carries across supervisor cycles.
func TestEvaluateExits_PeakTrackerPersists(t *testing.T) {
	cfg := testExitCfg()
	positions := []live.LivePosition{testPos("ETHUSDT", 1.0, 1000.0, 5)}
	pt := NewPeakTracker()

	// Cycle 1: price +25% → trail activates, peak = 1250
	EvaluateExits(cfg, positions, map[string]float64{"ETHUSDT": 1250.0}, pt)
	if !pt.TrailActive["ETHUSDT"] {
		t.Fatal("trail should be active after +25%")
	}

	// Cycle 2: price drops to trail stop territory
	// trail_stop = 1250 * (1 - 0.08) = 1150; 1140 <= 1150 → TRAILING_STOP
	result := EvaluateExits(cfg, positions, map[string]float64{"ETHUSDT": 1140.0}, pt)
	if result[0].Action != ExitTrailingStop {
		t.Errorf("expected TRAILING_STOP after peak persists, got %s", result[0].Action)
	}
}
