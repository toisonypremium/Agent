package storage

import (
	"btc-agent/internal/exchange/live"
	"testing"
	"time"
)

func TestHermesLossProtectionSnapshot(t *testing.T) {
	db, err := Open(t.TempDir() + "/loss.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Unix()
	seed := func(id, sym, side string, qty, price float64, ts int64) {
		_, e := db.Exec(`INSERT INTO live_orders(client_order_id,inst_id,symbol,side,type,price,quantity,notional,status,source) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, sym[:len(sym)-4]+"-USDT", sym, side, "limit", price, qty, price*qty, live.StatusFilled, "HERMES_OPERATOR")
		if e != nil {
			t.Fatal(e)
		}
		if e = db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: ts, ClientOrderID: id, Symbol: sym, Side: side, DeltaQuantity: qty, FillPrice: price, NotionalDelta: qty * price, FeeCurrency: "USDT"}); e != nil {
			t.Fatal(e)
		}
	}
	seed("b1", "ETHUSDT", "BUY", 1, 100, now-100)
	seed("s1", "ETHUSDT", "SELL", 1, 90, now-90)
	seed("b2", "SOLUSDT", "BUY", 1, 100, now-80)
	seed("s2", "SOLUSDT", "SELL", 1, 80, now-70)
	got, e := db.HermesLossProtectionSnapshot(time.Unix(now-200, 0))
	if e != nil {
		t.Fatal(e)
	}
	if got.ConsecutiveLosses != 2 || got.RollingRealizedPnL != -30 || got.ClosedSellFills != 2 {
		t.Fatalf("bad loss snapshot: %+v", got)
	}
	eth := got.BySymbol["ETHUSDT"]
	if eth.ClosedFills != 1 || eth.WinningFills != 0 || eth.LosingFills != 1 || eth.WinRate != 0 || eth.Expectancy != -0.10 {
		t.Fatalf("bad ETH performance: %+v", eth)
	}
	seed("b3", "RENDERUSDT", "BUY", 1, 100, now-60)
	seed("s3", "RENDERUSDT", "SELL", 1, 120, now-50)
	got, e = db.HermesLossProtectionSnapshot(time.Unix(now-200, 0))
	if e != nil {
		t.Fatal(e)
	}
	if got.ConsecutiveLosses != 0 || got.RollingRealizedPnL != -10 {
		t.Fatalf("profit did not reset streak: %+v", got)
	}
	render := got.BySymbol["RENDERUSDT"]
	if render.ClosedFills != 1 || render.WinningFills != 1 || render.WinRate != 1 || render.Expectancy != 0.20 {
		t.Fatalf("bad RENDER performance: %+v", render)
	}
}

func TestHermesLossProtectionDrawdownAndPreWindowEntry(t *testing.T) {
	db, err := Open(t.TempDir() + "/dd.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Unix()
	seed := func(id, sym, side string, price float64, ts int64) {
		_, e := db.Exec(`INSERT INTO live_orders(client_order_id,inst_id,symbol,side,type,price,quantity,notional,status,source) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "ETH-USDT", sym, side, "limit", price, 1, price, live.StatusFilled, "HERMES_OPERATOR")
		if e != nil {
			t.Fatal(e)
		}
		if e = db.SaveLivePositionEvent(live.LivePositionEvent{Timestamp: ts, ClientOrderID: id, Symbol: sym, Side: side, DeltaQuantity: 1, FillPrice: price, NotionalDelta: price, FeeCurrency: "USDT"}); e != nil {
			t.Fatal(e)
		}
	}
	seed("b0", "ETHUSDT", "BUY", 100, now-1000)
	seed("s0", "ETHUSDT", "SELL", 130, now-90)
	seed("b1", "SOLUSDT", "BUY", 100, now-80)
	seed("s1", "SOLUSDT", "SELL", 80, now-70)
	got, e := db.HermesLossProtectionSnapshot(time.Unix(now-100, 0))
	if e != nil {
		t.Fatal(e)
	}
	if got.RollingRealizedPnL != 10 || got.PeakRealizedPnL != 30 || got.RealizedDrawdown != 20 || got.MaxDrawdown != 20 || got.ClosedSellFills != 2 {
		t.Fatalf("bad drawdown replay: %+v", got)
	}
}
