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
