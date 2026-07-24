package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/liveguard"
	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

type RuntimeEvent struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	PayloadJSON string    `json:"payload_json,omitempty"`
	HandledAt   time.Time `json:"handled_at,omitempty"`
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Disable mmap for proot compatibility (avoids "out of memory" on Android/proot).
	// Keep one SQLite writer and wait briefly on locks so scheduler restart/live-doctor
	// does not fail just because the previous process is finishing a transaction.
	dsn := "file:" + path + "?_pragma=mmap_size(0)&_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=temp_store(MEMORY)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	d := &DB{db}
	return d, d.Migrate()
}
func (d *DB) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS candles(symbol TEXT, interval TEXT, open_time INTEGER, open REAL, high REAL, low REAL, close REAL, volume REAL, close_time INTEGER, PRIMARY KEY(symbol,interval,open_time));`,
		`CREATE TABLE IF NOT EXISTS indicators(symbol TEXT, interval TEXT, open_time INTEGER, ema20 REAL, ema50 REAL, ema200 REAL, rsi14 REAL, macd REAL, macd_signal REAL, macd_hist REAL, atr14 REAL, volume_ma20 REAL, PRIMARY KEY(symbol,interval,open_time));`,
		`CREATE TABLE IF NOT EXISTS market_analyses(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, btc_price REAL, regime TEXT, action_permission TEXT, risk_level TEXT, falling_knife_risk TEXT, fomo_risk TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS accumulation_plans(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, state TEXT, action_permission TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS paper_orders(id TEXT PRIMARY KEY, timestamp INTEGER, symbol TEXT, side TEXT, layer INTEGER, price REAL, quantity REAL, notional REAL, status TEXT, expires_at INTEGER, invalidation_price REAL, reason TEXT);`,
		`CREATE TABLE IF NOT EXISTS reports(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, type TEXT, content TEXT);`,
		`CREATE TABLE IF NOT EXISTS runtime_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, source TEXT, type TEXT, severity TEXT, fingerprint TEXT, payload_json TEXT, handled_at INTEGER);`,
		`CREATE TABLE IF NOT EXISTS microstructure_snapshots(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, symbol TEXT, source TEXT, status TEXT, fingerprint TEXT, payload_json TEXT);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_microstructure_symbol_timestamp ON microstructure_snapshots(symbol, timestamp);`,
		`CREATE TABLE IF NOT EXISTS performance_reviews(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_orders(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, type TEXT, price REAL, quantity REAL, notional REAL, status TEXT, submitted_at INTEGER, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_order_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_fills(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, filled_quantity REAL, avg_price REAL, fee REAL, fee_currency TEXT, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_positions(symbol TEXT PRIMARY KEY, inst_id TEXT, quantity REAL, avg_entry_price REAL, cost_basis REAL, fee_total REAL, fee_currency TEXT, updated_at INTEGER, opened_at INTEGER NOT NULL DEFAULT 0, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_position_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, delta_quantity REAL, fill_price REAL, notional_delta REAL, fee_delta REAL, fee_currency TEXT, position_qty REAL, avg_entry_price REAL, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS control_plane_proposals(decision_id TEXT PRIMARY KEY, caller TEXT NOT NULL, received_at INTEGER NOT NULL, payload_sha256 TEXT NOT NULL, payload_json TEXT NOT NULL, schema_verdict TEXT NOT NULL, policy_verdict TEXT NOT NULL, execution_verdict TEXT NOT NULL, reasons_json TEXT NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_control_plane_proposals_received_at ON control_plane_proposals(received_at DESC);`,
		`CREATE TABLE IF NOT EXISTS operator_change_requests(id TEXT PRIMARY KEY, action TEXT NOT NULL, requester TEXT NOT NULL, status TEXT NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL, payload_json TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS operator_change_confirmations(id INTEGER PRIMARY KEY AUTOINCREMENT, request_id TEXT NOT NULL, identity TEXT NOT NULL, confirmed_at INTEGER NOT NULL, UNIQUE(request_id, identity));`,
		`CREATE TABLE IF NOT EXISTS operator_audit_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER NOT NULL, identity TEXT NOT NULL, action TEXT NOT NULL, result TEXT NOT NULL, request_id TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '{}');`,
		`CREATE INDEX IF NOT EXISTS idx_operator_audit_timestamp ON operator_audit_events(timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS operator_settings(key TEXT PRIMARY KEY, value TEXT);`,
		`CREATE TABLE IF NOT EXISTS web_halt_requests(idempotency_hash TEXT PRIMARY KEY, identity TEXT NOT NULL, reason TEXT NOT NULL, created_at INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS execution_leases(name TEXT PRIMARY KEY, instance_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, acquired_at INTEGER NOT NULL, expires_at INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS llm_usage_events(request_id TEXT PRIMARY KEY, timestamp INTEGER NOT NULL, purpose TEXT NOT NULL, trigger_source TEXT NOT NULL DEFAULT '', trigger_reason TEXT NOT NULL DEFAULT '', model TEXT NOT NULL, prompt_tokens INTEGER, completion_tokens INTEGER, total_tokens INTEGER, usage_available INTEGER NOT NULL, latency_ms INTEGER NOT NULL, status TEXT NOT NULL, error_class TEXT NOT NULL DEFAULT '', state_hash TEXT NOT NULL DEFAULT '');`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_timestamp ON llm_usage_events(timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_purpose_timestamp ON llm_usage_events(purpose,timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS llm_call_reservations(purpose TEXT NOT NULL, state_hash TEXT NOT NULL, status TEXT NOT NULL, reserved_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0, next_retry_at INTEGER NOT NULL DEFAULT 0, valid_until INTEGER NOT NULL DEFAULT 0, error_class TEXT NOT NULL DEFAULT '', PRIMARY KEY(purpose,state_hash));`,
		`CREATE INDEX IF NOT EXISTS idx_llm_call_reservation_status ON llm_call_reservations(status,valid_until,next_retry_at);`,
		`CREATE TABLE IF NOT EXISTS hermes_runtime_state(key TEXT PRIMARY KEY, updated_at INTEGER NOT NULL, payload_json TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS hermes_execution_receipts(decision_id TEXT PRIMARY KEY, payload_hash TEXT NOT NULL, status TEXT NOT NULL, reserved_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, detail TEXT NOT NULL DEFAULT '');`,
		`CREATE INDEX IF NOT EXISTS idx_hermes_execution_receipts_updated_at ON hermes_execution_receipts(updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS hermes_exit_peaks(symbol TEXT PRIMARY KEY, peak REAL NOT NULL, trail_active INTEGER NOT NULL, updated_at INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS execution_markouts(event_id INTEGER NOT NULL, horizon_minutes INTEGER NOT NULL, mark_price REAL NOT NULL, markout_pct REAL NOT NULL, measured_at INTEGER NOT NULL, PRIMARY KEY(event_id,horizon_minutes));`,
		`CREATE TABLE IF NOT EXISTS hermes_managed_holdings(symbol TEXT PRIMARY KEY, inst_id TEXT NOT NULL, quantity REAL NOT NULL, avg_entry_price REAL NOT NULL, adopted_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, source TEXT NOT NULL, payload_json TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS thesis_capital_ledgers(thesis_id TEXT PRIMARY KEY, symbol TEXT NOT NULL, max_exposure_usdt REAL NOT NULL CHECK(max_exposure_usdt >= 0), reserved_usdt REAL NOT NULL DEFAULT 0 CHECK(reserved_usdt >= 0), filled_usdt REAL NOT NULL DEFAULT 0 CHECK(filled_usdt >= 0), remaining_dca_usdt REAL NOT NULL DEFAULT 0 CHECK(remaining_dca_usdt >= 0), status TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, payload_json TEXT NOT NULL DEFAULT '{}', CHECK(filled_usdt + reserved_usdt + remaining_dca_usdt <= max_exposure_usdt + 0.000000001));`,
		`CREATE TABLE IF NOT EXISTS thesis_capital_events(event_key TEXT PRIMARY KEY, thesis_id TEXT NOT NULL, client_order_id TEXT NOT NULL, event_type TEXT NOT NULL, notional_usdt REAL NOT NULL CHECK(notional_usdt >= 0), created_at INTEGER NOT NULL, payload_json TEXT NOT NULL DEFAULT '{}');`,
		`CREATE TABLE IF NOT EXISTS thesis_position_lifecycles(thesis_id TEXT PRIMARY KEY, symbol TEXT NOT NULL, state TEXT NOT NULL, invalidation_price REAL NOT NULL DEFAULT 0 CHECK(invalidation_price >= 0), primary_target_price REAL NOT NULL DEFAULT 0 CHECK(primary_target_price >= 0), protection_price REAL NOT NULL DEFAULT 0 CHECK(protection_price >= 0), position_quantity REAL NOT NULL DEFAULT 0 CHECK(position_quantity >= 0), avg_entry_price REAL NOT NULL DEFAULT 0 CHECK(avg_entry_price >= 0), opened_at INTEGER NOT NULL DEFAULT 0, last_evaluated_at INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, payload_json TEXT NOT NULL DEFAULT '{}');`,
		`CREATE INDEX IF NOT EXISTS idx_thesis_capital_events_thesis_created ON thesis_capital_events(thesis_id, created_at);`,
		`INSERT OR IGNORE INTO operator_settings(key, value) VALUES('halted', 'true');`}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return err
		}
	}
	for _, col := range []struct {
		name string
		def  string
	}{
		{"layer_index", "INTEGER"},
		{"source", "TEXT"},
		{"invalidation_price", "REAL"},
		{"expires_at", "INTEGER"},
		{"decision_reason", "TEXT"},
		{"last_management_action", "TEXT"},
		{"decision_id", "TEXT"},
		{"intent", "TEXT"},
		{"strategy_version", "TEXT"},
		{"config_hash", "TEXT"},
		{"thesis_id", "TEXT"},
	} {
		if err := d.ensureColumn("live_orders", col.name, col.def); err != nil {
			return err
		}
	}
	if err := d.ensureColumn("paper_orders", "closed_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("live_positions", "opened_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	for _, table := range []string{"live_fills", "live_position_events", "live_positions", "hermes_managed_holdings"} {
		if err := d.ensureColumn(table, "thesis_id", "TEXT"); err != nil {
			return err
		}
	}
	if _, err := d.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_live_orders_active_logical_intent ON live_orders(symbol, layer_index, side, source) WHERE status IN ('PLANNED','SUBMITTED','PARTIAL_FILL','LIVE_OPEN','PARTIALLY_FILLED','UNKNOWN_NEEDS_MANUAL_CHECK');`); err != nil {
		return err
	}
	return nil
}

func (d *DB) ensureColumn(table, name, def string) error {
	rows, err := d.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var colName, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if colName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = d.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + def)
	return err
}
func (d *DB) SaveManagedCycleReport(result liveguard.ManagedCycleResult) error {
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return d.SaveReport("auto_live_management", string(b))
}

// OpenReadOnly opens an existing SQLite database without running migrations or
// creating parent directories. It is for observers such as the Web Console;
// callers cannot use it to bootstrap or mutate runtime state.
func OpenReadOnly(path string) (*DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sqlite read-only path is not a regular file: %s", path)
	}
	dsn := "file:" + path + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(ON)&_pragma=mmap_size(0)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return &DB{db}, nil
}

// OpenWritableExisting opens an existing runtime database without migrations.
// It is reserved for the narrow audited halt bridge; callers must not use it
// for general execution, configuration, or scheduler writes.
func OpenWritableExisting(path string) (*DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sqlite writable path is not a regular file: %s", path)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(wal)&_pragma=synchronous(NORMAL)&_pragma=temp_store(MEMORY"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return &DB{db}, nil
}
