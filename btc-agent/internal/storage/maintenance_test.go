package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneMaintenanceDeletesOldRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Unix(1700000000, 0)
	old := now.AddDate(0, 0, -40).Unix()
	recent := now.AddDate(0, 0, -5).Unix()

	mustExec(t, db, `INSERT INTO reports(timestamp,type,content) VALUES(?,?,?)`, old, "old", "old")
	mustExec(t, db, `INSERT INTO reports(timestamp,type,content) VALUES(?,?,?)`, recent, "recent", "recent")
	mustExec(t, db, `INSERT INTO live_order_events(timestamp,client_order_id,order_id,status,payload_json) VALUES(?,?,?,?,?)`, old, "old", "1", "LIVE_OPEN", "{}")
	mustExec(t, db, `INSERT INTO live_order_events(timestamp,client_order_id,order_id,status,payload_json) VALUES(?,?,?,?,?)`, recent, "recent", "2", "LIVE_OPEN", "{}")
	mustExec(t, db, `INSERT INTO live_position_events(timestamp,client_order_id,order_id,inst_id,symbol,side,delta_quantity,fill_price,notional_delta,fee_delta,fee_currency,position_qty,avg_entry_price,status,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, old, "old", "1", "BTC-USDT", "BTCUSDT", "BUY", 1, 1, 1, 0, "USDT", 1, 1, "FILLED", "{}")
	mustExec(t, db, `INSERT INTO live_position_events(timestamp,client_order_id,order_id,inst_id,symbol,side,delta_quantity,fill_price,notional_delta,fee_delta,fee_currency,position_qty,avg_entry_price,status,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, recent, "recent", "2", "BTC-USDT", "BTCUSDT", "BUY", 1, 1, 1, 0, "USDT", 1, 1, "FILLED", "{}")

	result, err := db.PruneMaintenance(MaintenanceConfig{ReportRetentionDays: 30, EventRetentionDays: 30, MaxClosedPaperOrders: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReportsDeleted != 1 || result.LiveOrderEventsDeleted != 1 || result.LivePositionEventsDeleted != 1 {
		t.Fatalf("unexpected deleted counts: %+v", result)
	}
	if got := countRows(t, db, "reports"); got != 1 {
		t.Fatalf("reports count=%d want 1", got)
	}
	if got := countRows(t, db, "live_order_events"); got != 1 {
		t.Fatalf("live_order_events count=%d want 1", got)
	}
	if got := countRows(t, db, "live_position_events"); got != 1 {
		t.Fatalf("live_position_events count=%d want 1", got)
	}
}

func TestPruneMaintenanceCapsClosedPaperOrdersAndKeepsOpen(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		mustExec(t, db, `INSERT INTO paper_orders(id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("closed-%d", i), int64(100+i), "ETHUSDT", "BUY", 1, 1, 1, 1, "FILLED", int64(200+i), 0, "test")
	}
	mustExec(t, db, `INSERT INTO paper_orders(id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, "open-1", int64(50), "ETHUSDT", "BUY", 1, 1, 1, 1, "OPEN", int64(200), 0, "test")

	result, err := db.PruneMaintenance(MaintenanceConfig{ReportRetentionDays: 30, EventRetentionDays: 30, MaxClosedPaperOrders: 2}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosedPaperOrdersDeleted != 3 {
		t.Fatalf("closed deleted=%d want 3", result.ClosedPaperOrdersDeleted)
	}
	if got := countRowsWhere(t, db, "paper_orders", "status='OPEN'"); got != 1 {
		t.Fatalf("open orders=%d want 1", got)
	}
	if got := countRowsWhere(t, db, "paper_orders", "status<>'OPEN'"); got != 2 {
		t.Fatalf("closed orders=%d want 2", got)
	}
}

func TestPruneMaintenanceCapsCandlesAnalysesAndPlans(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		mustExec(t, db, `INSERT INTO candles(symbol,interval,open_time,open,high,low,close,volume,close_time) VALUES(?,?,?,?,?,?,?,?,?)`, "BTCUSDT", "1d", int64(100+i), 1, 1, 1, 1, 1, int64(200+i))
		mustExec(t, db, `INSERT INTO candles(symbol,interval,open_time,open,high,low,close,volume,close_time) VALUES(?,?,?,?,?,?,?,?,?)`, "ETHUSDT", "1d", int64(100+i), 1, 1, 1, 1, 1, int64(200+i))
		mustExec(t, db, `INSERT INTO market_analyses(timestamp,btc_price,regime,action_permission,risk_level,falling_knife_risk,fomo_risk,payload_json) VALUES(?,?,?,?,?,?,?,?)`, int64(100+i), 1, "RANGE", "WATCH", "MEDIUM", "LOW", "LOW", "{}")
		mustExec(t, db, `INSERT INTO accumulation_plans(timestamp,state,action_permission,payload_json) VALUES(?,?,?,?)`, int64(100+i), "WATCH", "WATCH", "{}")
	}

	result, err := db.PruneMaintenance(MaintenanceConfig{MaxCandlesPerSymbolInterval: 3, MaxAnalysisRows: 2, MaxPlanRows: 2}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if result.CandlesDeleted != 4 || result.AnalysesDeleted != 3 || result.PlansDeleted != 3 {
		t.Fatalf("unexpected deleted counts: %+v", result)
	}
	if got := countRowsWhere(t, db, "candles", "symbol='BTCUSDT'"); got != 3 {
		t.Fatalf("btc candles=%d want 3", got)
	}
	if got := countRowsWhere(t, db, "candles", "symbol='ETHUSDT'"); got != 3 {
		t.Fatalf("eth candles=%d want 3", got)
	}
	if got := countRows(t, db, "market_analyses"); got != 2 {
		t.Fatalf("analyses=%d want 2", got)
	}
	if got := countRows(t, db, "accumulation_plans"); got != 2 {
		t.Fatalf("plans=%d want 2", got)
	}
}

func TestCloseStaleOpenLiveOrders(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Unix(1700000000, 0)

	// Case 1: has expires_at in the past — should be closed.
	mustExec(t, db, `INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"c1", "o1", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, 0.01, 1, "LIVE_OPEN", now.Add(-10*24*time.Hour).Unix(), now.Unix(), now.Add(-1*time.Hour).Unix())

	// Case 2: no expires_at and submitted_at is older than staledays — should be closed.
	mustExec(t, db, `INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"c2", "o2", "SOL-USDT", "SOLUSDT", "BUY", "limit", 20, 0.5, 10, "SUBMITTED", now.Add(-10*24*time.Hour).Unix(), now.Unix(), 0)

	// Case 3: recent order with no expires_at — should NOT be closed.
	mustExec(t, db, `INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"c3", "o3", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, 0.01, 1, "LIVE_OPEN", now.Add(-1*24*time.Hour).Unix(), now.Unix(), 0)

	// Case 4: future expires_at — should NOT be closed.
	mustExec(t, db, `INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"c4", "o4", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, 0.01, 1, "LIVE_OPEN", now.Add(-2*24*time.Hour).Unix(), now.Unix(), now.Add(24*time.Hour).Unix())

	result, err := db.PruneMaintenance(MaintenanceConfig{
		StaleOpenLiveOrderDays: 7,
	}, now)
	if err != nil {
		t.Fatalf("PruneMaintenance: %v", err)
	}
	if result.StaleOpenLiveOrdersClosed != 2 {
		t.Fatalf("stale_closed=%d want 2", result.StaleOpenLiveOrdersClosed)
	}
	// c3 and c4 remain open.
	if got := countRowsWhere(t, db, "live_orders", "status IN ('LIVE_OPEN','SUBMITTED','PLANNED','PARTIAL_FILL')"); got != 2 {
		t.Fatalf("remaining_open=%d want 2", got)
	}
	if got := countRowsWhere(t, db, "live_orders", "status='EXPIRED'"); got != 2 {
		t.Fatalf("expired=%d want 2", got)
	}
	if got := countRowsWhere(t, db, "live_orders", "last_management_action='maintenance_stale_expire'"); got != 2 {
		t.Fatalf("action_tagged=%d want 2", got)
	}
}

func TestCloseStaleOpenLiveOrdersDisabledWhenZero(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Unix(1700000000, 0)
	mustExec(t, db, `INSERT INTO live_orders(client_order_id,order_id,inst_id,symbol,side,type,price,quantity,notional,status,submitted_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"c1", "o1", "ETH-USDT", "ETHUSDT", "BUY", "limit", 100, 0.01, 1, "LIVE_OPEN", now.Add(-30*24*time.Hour).Unix(), now.Unix(), 0)

	result, err := db.PruneMaintenance(MaintenanceConfig{StaleOpenLiveOrderDays: -1}, now)
	if err != nil {
		t.Fatalf("PruneMaintenance: %v", err)
	}
	if result.StaleOpenLiveOrdersClosed != 0 {
		t.Fatalf("stale_closed=%d want 0 (disabled)", result.StaleOpenLiveOrdersClosed)
	}
}

func mustExec(t *testing.T, db *DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *DB, table string) int {
	t.Helper()
	return countRowsWhere(t, db, table, "1=1")
}

func countRowsWhere(t *testing.T, db *DB, table, where string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + where).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
