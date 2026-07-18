package main

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

func TestApplyPortfolioLossState(t *testing.T) {
	now := time.Unix(1800000000, 0)
	cfg := config.Config{}
	cfg.Risk.MaxTotalEquityDrawdownPct = 0.10
	cfg.Live.ManagementIntervalMinutes = 5

	t.Run("missing remains unknown", func(t *testing.T) {
		db := openPortfolioLossTestDB(t)
		var got liveguard.ManagedExecutionContext
		applyPortfolioLossState(cfg, db, &got, now)
		if got.PortfolioLossStateKnown || got.PortfolioLossLockActive {
			t.Fatalf("missing state must fail closed as unknown: %+v", got)
		}
	})

	t.Run("fresh safe state is known inactive", func(t *testing.T) {
		db := openPortfolioLossTestDB(t)
		if _, err := db.UpdateEquityRiskState(100, now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpdateEquityRiskState(95, now); err != nil {
			t.Fatal(err)
		}
		var got liveguard.ManagedExecutionContext
		applyPortfolioLossState(cfg, db, &got, now)
		if !got.PortfolioLossStateKnown || got.PortfolioLossLockActive || got.PortfolioLossDrawdownPct != 0.05 {
			t.Fatalf("fresh safe state wrong: %+v", got)
		}
	})

	t.Run("fresh breached state is active", func(t *testing.T) {
		db := openPortfolioLossTestDB(t)
		if _, err := db.UpdateEquityRiskState(100, now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpdateEquityRiskState(80, now); err != nil {
			t.Fatal(err)
		}
		var got liveguard.ManagedExecutionContext
		applyPortfolioLossState(cfg, db, &got, now)
		if !got.PortfolioLossStateKnown || !got.PortfolioLossLockActive || got.PortfolioLossDrawdownPct != 0.20 {
			t.Fatalf("fresh breached state wrong: %+v", got)
		}
	})

	t.Run("stale remains unknown", func(t *testing.T) {
		db := openPortfolioLossTestDB(t)
		if _, err := db.UpdateEquityRiskState(100, now.Add(-16*time.Minute)); err != nil {
			t.Fatal(err)
		}
		var got liveguard.ManagedExecutionContext
		applyPortfolioLossState(cfg, db, &got, now)
		if got.PortfolioLossStateKnown {
			t.Fatalf("stale state must fail closed as unknown: %+v", got)
		}
	})

	t.Run("disabled guard is known inactive", func(t *testing.T) {
		db := openPortfolioLossTestDB(t)
		disabled := cfg
		disabled.Risk.MaxTotalEquityDrawdownPct = 0
		var got liveguard.ManagedExecutionContext
		applyPortfolioLossState(disabled, db, &got, now)
		if !got.PortfolioLossStateKnown || got.PortfolioLossLockActive {
			t.Fatalf("disabled guard state wrong: %+v", got)
		}
	})
}

func TestApplyPortfolioLossStateDailyRealizedLoss(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	cfg := config.Config{}
	cfg.Risk.MaxDailyRealizedLossPct = 0.03
	cfg.Live.ManagementIntervalMinutes = 5
	db := openPortfolioLossTestDB(t)
	if _, err := db.UpdateEquityRiskState(100, now); err != nil {
		t.Fatal(err)
	}
	insert := func(id, side, source string, ts int64, price float64) {
		_, err := db.Exec(`INSERT INTO live_orders(client_order_id,inst_id,symbol,side,type,price,quantity,notional,status,source) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "ETH-USDT", "ETHUSDT", side, "limit", price, 1, price, "FILLED", source)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: ts, ClientOrderID: id, InstID: "ETH-USDT", Symbol: "ETHUSDT", Side: side, DeltaQuantity: 1, FillPrice: price, NotionalDelta: price, FeeCurrency: "USDT"}); err != nil {
			t.Fatal(err)
		}
	}
	insert("normal-buy", "BUY", "deterministic_agent2_layer_1", now.Add(-2*time.Hour).Unix(), 100)
	insert("normal-sell", "SELL", "deterministic_agent2_layer_1", now.Add(-time.Hour).Unix(), 96)
	var got liveguard.ManagedExecutionContext
	applyPortfolioLossState(cfg, db, &got, now)
	if !got.PortfolioLossStateKnown || !got.DailyRealizedLossLockActive || got.DailyRealizedPnL != -4 || got.DailyLossEquityBasis != 100 {
		t.Fatalf("normal managed daily loss must lock BUY: %+v", got)
	}
	if _, err := db.UpdateEquityRiskState(80, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var stable liveguard.ManagedExecutionContext
	applyPortfolioLossState(cfg, db, &stable, now.Add(time.Minute))
	if stable.DailyLossEquityBasis != 100 {
		t.Fatalf("daily loss basis changed with intraday equity: %+v", stable)
	}
	// A prior UTC day must not count toward today's cap.
	insert("old-buy", "BUY", "deterministic_agent2_layer_1", now.Add(-26*time.Hour).Unix(), 100)
	insert("old-sell", "SELL", "deterministic_agent2_layer_1", now.Add(-25*time.Hour).Unix(), 50)
	var next liveguard.ManagedExecutionContext
	applyPortfolioLossState(cfg, db, &next, now.Add(time.Minute))
	if next.DailyRealizedPnL != -4 || !next.DailyRealizedLossLockActive {
		t.Fatalf("daily UTC boundary wrong: %+v", next)
	}
}

func openPortfolioLossTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "loss-state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
