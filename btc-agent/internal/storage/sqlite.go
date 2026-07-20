package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/runtime/outbox"
	"btc-agent/internal/runtime/ownership"
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
		`CREATE TABLE IF NOT EXISTS execution_leases(name TEXT PRIMARY KEY, instance_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, acquired_at INTEGER NOT NULL, expires_at INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS outbox_events(id TEXT PRIMARY KEY, event_type TEXT NOT NULL, destination TEXT NOT NULL, payload BLOB NOT NULL, idempotency_key TEXT NOT NULL UNIQUE, status TEXT NOT NULL, retry_count INTEGER NOT NULL DEFAULT 0, next_retry_at INTEGER NOT NULL, last_error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_claim ON outbox_events(status, next_retry_at, created_at);`,
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

func (d *DB) SaveLiveOrderFromParams(clientOrderID, orderID, instID, symbol, side, ordType string, price, quantity, notional float64, status string) error {
	return d.SaveManagedLiveOrder(clientOrderID, orderID, instID, symbol, side, ordType, price, quantity, notional, status, live.OrderStatus{})
}

func (d *DB) SaveManagedLiveOrder(clientOrderID, orderID, instID, symbol, side, ordType string, price, quantity, notional float64, status string, meta live.OrderStatus) error {
	now := time.Now().Unix()
	_, err := d.Exec(
		`INSERT OR REPLACE INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		clientOrderID, orderID, instID, symbol, side, ordType, price, quantity, notional, status, now, now, meta.LayerIndex, meta.Source, meta.InvalidationPrice, meta.ExpiresAt, meta.DecisionReason, meta.LastManagementAction,
	)
	return err
}

func (d *DB) ReserveManagedLiveOrder(clientOrderID string, desired liveguard.ManagedDesiredOrder, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	now := time.Now().Unix()
	expiresAt := int64(0)
	if !desired.ExpiresAt.IsZero() {
		expiresAt = desired.ExpiresAt.Unix()
	}
	_, err := d.Exec(
		`INSERT INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		clientOrderID, "", desired.InstID, desired.Symbol, desired.Side, desired.Type, desired.Price, desired.Quantity, desired.Notional, live.StatusPlanned, now, now, desired.LayerIndex, desired.Source, desired.InvalidationPrice, expiresAt, desired.DecisionReason, "planned: "+reason,
	)
	return err
}

func (d *DB) MarkManagedLiveOrderSubmitted(clientOrderID string, result live.OrderResult) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	res, err := d.Exec(`UPDATE live_orders SET order_id=CASE WHEN ?<>'' THEN ? ELSE order_id END, inst_id=CASE WHEN ?<>'' THEN ? ELSE inst_id END, status=?, updated_at=?, last_management_action=? WHERE client_order_id=?`, result.OrderID, result.OrderID, result.InstID, result.InstID, live.StatusSubmitted, time.Now().Unix(), "submitted", clientOrderID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("live order reservation not found: client_order_id=%q", clientOrderID)
	}
	return nil
}

func (d *DB) MarkManagedLiveOrderRejected(clientOrderID string, reason string) error {
	if clientOrderID == "" {
		return fmt.Errorf("client_order_id required")
	}
	res, err := d.Exec(`UPDATE live_orders SET status=?, updated_at=?, last_management_action=? WHERE client_order_id=?`, live.StatusRejected, time.Now().Unix(), "rejected: "+reason, clientOrderID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("live order reservation not found: client_order_id=%q", clientOrderID)
	}
	return nil
}

func (d *DB) OpenLiveOrders() ([]live.OrderStatus, error) {
	return d.OpenLiveOrdersDetailed()
}

func (d *DB) OpenLiveOrdersDetailed() ([]live.OrderStatus, error) {
	rows, err := d.Query(`SELECT client_order_id, order_id, inst_id, symbol, type, side, price, quantity, notional, status, submitted_at, updated_at, layer_index, source, invalidation_price, expires_at, decision_reason, last_management_action FROM live_orders WHERE status IN ('PLANNED', 'SUBMITTED', 'PARTIAL_FILL', 'LIVE_OPEN', 'PARTIALLY_FILLED')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.OrderStatus{}
	for rows.Next() {
		var o live.OrderStatus
		var symbol, source, decisionReason, lastAction sql.NullString
		var notional, invalidation sql.NullFloat64
		var submittedAt, layerIndex, expiresAt sql.NullInt64
		if err := rows.Scan(&o.ClientOrderID, &o.OrderID, &o.InstID, &symbol, &o.OrderType, &o.Side, &o.Price, &o.Quantity, &notional, &o.Status, &submittedAt, &o.UpdatedAt, &layerIndex, &source, &invalidation, &expiresAt, &decisionReason, &lastAction); err != nil {
			return nil, err
		}
		if symbol.Valid {
			o.Symbol = symbol.String
		}
		if notional.Valid {
			o.Notional = notional.Float64
		}
		if submittedAt.Valid {
			o.SubmittedAt = submittedAt.Int64
		}
		if layerIndex.Valid {
			o.LayerIndex = int(layerIndex.Int64)
		}
		if source.Valid {
			o.Source = source.String
		}
		if invalidation.Valid {
			o.InvalidationPrice = invalidation.Float64
		}
		if expiresAt.Valid {
			o.ExpiresAt = expiresAt.Int64
		}
		if decisionReason.Valid {
			o.DecisionReason = decisionReason.String
		}
		if lastAction.Valid {
			o.LastManagementAction = lastAction.String
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (d *DB) SaveLiveOrderStatus(o live.OrderStatus) error {
	o.Status = live.NormalizeOrderStatus(o.Status)
	updatedAt := o.UpdatedAt
	if updatedAt == 0 {
		updatedAt = time.Now().Unix()
	}
	b, _ := json.Marshal(o)
	update := func(where string, id string) (int64, error) {
		res, err := d.Exec(
			`UPDATE live_orders SET
				order_id=CASE WHEN ?<>'' THEN ? ELSE order_id END,
				inst_id=CASE WHEN ?<>'' THEN ? ELSE inst_id END,
				side=CASE WHEN ?<>'' THEN ? ELSE side END,
				type=CASE WHEN ?<>'' THEN ? ELSE type END,
				price=CASE WHEN ?>0 THEN ? ELSE price END,
				quantity=CASE WHEN ?>0 THEN ? ELSE quantity END,
				notional=CASE WHEN ?>0 AND ?>0 THEN ?*? ELSE notional END,
				status=?, updated_at=?, payload_json=? `+where,
			o.OrderID, o.OrderID,
			o.InstID, o.InstID,
			o.Side, o.Side,
			o.OrderType, o.OrderType,
			o.Price, o.Price,
			o.Quantity, o.Quantity,
			o.Price, o.Quantity, o.Price, o.Quantity,
			o.Status, updatedAt, string(b), id,
		)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	if o.ClientOrderID != "" {
		rows, err := update(`WHERE client_order_id=?`, o.ClientOrderID)
		if err != nil || rows > 0 {
			return err
		}
	}
	if o.OrderID != "" {
		rows, err := update(`WHERE order_id=?`, o.OrderID)
		if err != nil || rows > 0 {
			return err
		}
	}
	return fmt.Errorf("live order status not found: client_order_id=%q order_id=%q", o.ClientOrderID, o.OrderID)
}

func (d *DB) SaveLiveOrderEvent(o live.OrderStatus) error {
	now := time.Now().Unix()
	b, _ := json.Marshal(o)
	_, err := d.Exec(
		`INSERT INTO live_order_events(timestamp, client_order_id, order_id, status, payload_json) VALUES(?,?,?,?,?)`,
		now, o.ClientOrderID, o.OrderID, o.Status, string(b),
	)
	return err
}

func (d *DB) LiveFillSnapshot(clientOrderID, orderID string) (live.LiveFillSnapshot, bool, error) {
	var fill live.LiveFillSnapshot
	var err error
	if clientOrderID != "" {
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at FROM live_fills WHERE client_order_id=?`, clientOrderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt)
	} else if orderID != "" {
		err = d.QueryRow(`SELECT client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at FROM live_fills WHERE order_id=?`, orderID).Scan(&fill.ClientOrderID, &fill.OrderID, &fill.InstID, &fill.Symbol, &fill.Side, &fill.FilledQuantity, &fill.AvgPrice, &fill.Fee, &fill.FeeCurrency, &fill.UpdatedAt)
	} else {
		return live.LiveFillSnapshot{}, false, nil
	}
	if err == sql.ErrNoRows {
		return live.LiveFillSnapshot{}, false, nil
	}
	if err != nil {
		return live.LiveFillSnapshot{}, false, err
	}
	return fill, true, nil
}

func (d *DB) SaveLiveFillSnapshot(fill live.LiveFillSnapshot) error {
	if fill.ClientOrderID == "" {
		return fmt.Errorf("live fill snapshot client_order_id required")
	}
	b, _ := json.Marshal(fill)
	_, err := d.Exec(`INSERT OR REPLACE INTO live_fills(client_order_id, order_id, inst_id, symbol, side, filled_quantity, avg_price, fee, fee_currency, updated_at, payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fill.ClientOrderID, fill.OrderID, fill.InstID, fill.Symbol, strings.ToUpper(fill.Side), fill.FilledQuantity, fill.AvgPrice, fill.Fee, strings.ToUpper(fill.FeeCurrency), fill.UpdatedAt, string(b))
	return err
}

func (d *DB) ApplyLivePositionEvent(event live.LivePositionEvent) (live.LivePosition, error) {
	if event.Symbol == "" {
		event.Symbol = live.InternalSymbol(event.InstID)
	}
	event.Side = strings.ToUpper(event.Side)
	event.FeeCurrency = strings.ToUpper(event.FeeCurrency)
	if event.Symbol == "" {
		return live.LivePosition{}, fmt.Errorf("live position event symbol required")
	}
	if event.DeltaQuantity <= 0 {
		return live.LivePosition{}, fmt.Errorf("live position event delta quantity must be positive")
	}
	if event.FillPrice <= 0 {
		return live.LivePosition{}, fmt.Errorf("live position event fill price must be positive")
	}

	tx, err := d.Begin()
	if err != nil {
		return live.LivePosition{}, err
	}
	defer tx.Rollback()

	pos, found, err := livePositionBySymbol(tx, event.Symbol)
	if err != nil {
		return live.LivePosition{}, err
	}
	if !found {
		pos = live.LivePosition{Symbol: event.Symbol, InstID: event.InstID}
	}
	if pos.InstID == "" {
		pos.InstID = event.InstID
	}

	switch event.Side {
	case "BUY":
		pos.Quantity += event.DeltaQuantity
		pos.CostBasis += event.NotionalDelta
	case "SELL":
		if pos.Quantity+1e-12 < event.DeltaQuantity {
			return live.LivePosition{}, fmt.Errorf("sell delta %.12f exceeds live position %.12f for %s", event.DeltaQuantity, pos.Quantity, event.Symbol)
		}
		avgCost := 0.0
		if pos.Quantity > 0 {
			avgCost = pos.CostBasis / pos.Quantity
		}
		pos.Quantity -= event.DeltaQuantity
		pos.CostBasis -= avgCost * event.DeltaQuantity
		if pos.Quantity < 1e-12 {
			pos.Quantity = 0
			pos.CostBasis = 0
		}
	default:
		return live.LivePosition{}, fmt.Errorf("unsupported live position side %q", event.Side)
	}
	if pos.Quantity > 0 {
		pos.AvgEntryPrice = pos.CostBasis / pos.Quantity
	} else {
		pos.AvgEntryPrice = 0
	}
	pos.FeeTotal += event.FeeDelta
	pos.FeeCurrency = mergeFeeCurrency(pos.FeeCurrency, event.FeeCurrency)
	if event.Timestamp > 0 {
		pos.UpdatedAt = event.Timestamp
	} else {
		pos.UpdatedAt = time.Now().Unix()
	}

	b, _ := json.Marshal(pos)
	_, err = tx.Exec(`INSERT OR REPLACE INTO live_positions(symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, payload_json) VALUES(?,?,?,?,?,?,?,?,?)`, pos.Symbol, pos.InstID, pos.Quantity, pos.AvgEntryPrice, pos.CostBasis, pos.FeeTotal, pos.FeeCurrency, pos.UpdatedAt, string(b))
	if err != nil {
		return live.LivePosition{}, err
	}
	if err := tx.Commit(); err != nil {
		return live.LivePosition{}, err
	}
	event.PositionQty = pos.Quantity
	event.AvgEntryPrice = pos.AvgEntryPrice
	return pos, nil
}

func livePositionBySymbol(q interface {
	QueryRow(string, ...any) *sql.Row
}, symbol string) (live.LivePosition, bool, error) {
	var pos live.LivePosition
	err := q.QueryRow(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at FROM live_positions WHERE symbol=?`, symbol).Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt)
	if err == sql.ErrNoRows {
		return live.LivePosition{}, false, nil
	}
	if err != nil {
		return live.LivePosition{}, false, err
	}
	return pos, true, nil
}

func (d *DB) SaveLivePositionEvent(event live.LivePositionEvent) error {
	b, _ := json.Marshal(event)
	_, err := d.Exec(`INSERT INTO live_position_events(timestamp, client_order_id, order_id, inst_id, symbol, side, delta_quantity, fill_price, notional_delta, fee_delta, fee_currency, position_qty, avg_entry_price, status, payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.Timestamp, event.ClientOrderID, event.OrderID, event.InstID, event.Symbol, strings.ToUpper(event.Side), event.DeltaQuantity, event.FillPrice, event.NotionalDelta, event.FeeDelta, strings.ToUpper(event.FeeCurrency), event.PositionQty, event.AvgEntryPrice, event.Status, string(b))
	return err
}

func (d *DB) LivePositions() ([]live.LivePosition, error) {
	rows, err := d.Query(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at FROM live_positions ORDER BY symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.LivePosition{}
	for rows.Next() {
		var pos live.LivePosition
		if err := rows.Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, pos)
	}
	return out, rows.Err()
}

func withSQLiteRetry(fn func() error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = fn()
		if err == nil || !isSQLiteBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	return err
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy") || strings.Contains(lower, "database table is locked")
}

func mergeFeeCurrency(existing, incoming string) string {
	existing = strings.ToUpper(existing)
	incoming = strings.ToUpper(incoming)
	if existing == "" {
		return incoming
	}
	if incoming == "" || incoming == existing {
		return existing
	}
	return "MIXED"
}

func (d *DB) SetHaltStatus(halted bool) error {
	val := "false"
	if halted {
		val = "true"
	}
	_, err := d.Exec(`INSERT OR REPLACE INTO operator_settings(key, value) VALUES('halted', ?)`, val)
	return err
}

func (d *DB) IsHalted() (bool, error) {
	var val string
	err := d.QueryRow(`SELECT value FROM operator_settings WHERE key='halted'`).Scan(&val)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return true, err // Safe default on database errors
	}
	return val == "true", nil
}

// AcquireExecutionLease atomically takes an expired/free lease and advances its
// fencing token. The transaction prevents two local database users becoming owner.
func (d *DB) AcquireExecutionLease(ctx context.Context, name, instanceID string, now time.Time, ttl time.Duration) (ownership.Lease, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return ownership.Lease{}, false, err
	}
	defer tx.Rollback()
	var currentInstance string
	var fence, acquiredAt, expiresAt int64
	err = tx.QueryRowContext(ctx, `SELECT instance_id,fencing_token,acquired_at,expires_at FROM execution_leases WHERE name=?`, name).Scan(&currentInstance, &fence, &acquiredAt, &expiresAt)
	if err != nil && err != sql.ErrNoRows {
		return ownership.Lease{}, false, err
	}
	if err == nil && expiresAt > now.Unix() && currentInstance != instanceID {
		return ownership.Lease{}, false, nil
	}
	fence++
	if fence < 1 {
		fence = 1
	}
	lease := ownership.Lease{Name: name, InstanceID: instanceID, FencingToken: fence, AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_leases(name,instance_id,fencing_token,acquired_at,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET instance_id=excluded.instance_id,fencing_token=excluded.fencing_token,acquired_at=excluded.acquired_at,expires_at=excluded.expires_at`, name, instanceID, fence, lease.AcquiredAt.Unix(), lease.ExpiresAt.Unix())
	if err != nil {
		return ownership.Lease{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return ownership.Lease{}, false, err
	}
	return lease, true, nil
}

func (d *DB) RenewExecutionLease(ctx context.Context, lease ownership.Lease, now time.Time, ttl time.Duration) (ownership.Lease, bool, error) {
	next := now.Add(ttl)
	res, err := d.ExecContext(ctx, `UPDATE execution_leases SET expires_at=? WHERE name=? AND instance_id=? AND fencing_token=? AND expires_at>?`, next.Unix(), lease.Name, lease.InstanceID, lease.FencingToken, now.Unix())
	if err != nil {
		return ownership.Lease{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ownership.Lease{}, false, err
	}
	if n != 1 {
		return ownership.Lease{}, false, nil
	}
	lease.ExpiresAt = next
	return lease, true, nil
}

func (d *DB) ReleaseExecutionLease(ctx context.Context, lease ownership.Lease) error {
	_, err := d.ExecContext(ctx, `DELETE FROM execution_leases WHERE name=? AND instance_id=? AND fencing_token=?`, lease.Name, lease.InstanceID, lease.FencingToken)
	return err
}

func (d *DB) CurrentExecutionLease(ctx context.Context, name string) (ownership.Lease, bool, error) {
	var l ownership.Lease
	var acquired, expires int64
	err := d.QueryRowContext(ctx, `SELECT name,instance_id,fencing_token,acquired_at,expires_at FROM execution_leases WHERE name=?`, name).Scan(&l.Name, &l.InstanceID, &l.FencingToken, &acquired, &expires)
	if err == sql.ErrNoRows {
		return ownership.Lease{}, false, nil
	}
	if err != nil {
		return ownership.Lease{}, false, err
	}
	l.AcquiredAt = time.Unix(acquired, 0).UTC()
	l.ExpiresAt = time.Unix(expires, 0).UTC()
	return l, true, nil
}

func (d *DB) EnqueueOutbox(ctx context.Context, item outbox.Item) error {
	if item.ID == "" || item.EventType == "" || item.Destination == "" || item.IdempotencyKey == "" {
		return fmt.Errorf("outbox id, type, destination and idempotency key required")
	}
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	next := item.NextRetryAt
	if next.IsZero() {
		next = now
	}
	status := item.Status
	if status == "" {
		status = outbox.StatusPending
	}
	_, err := d.ExecContext(ctx, `INSERT INTO outbox_events(id,event_type,destination,payload,idempotency_key,status,retry_count,next_retry_at,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.EventType, item.Destination, item.Payload, item.IdempotencyKey, status, item.RetryCount, next.Unix(), item.LastError, now.Unix(), now.Unix())
	return err
}

func (d *DB) ClaimOutbox(ctx context.Context, now time.Time, limit int) ([]outbox.Item, error) {
	if limit < 1 {
		return []outbox.Item{}, nil
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,event_type,destination,payload,idempotency_key,status,retry_count,next_retry_at,last_error,created_at FROM outbox_events WHERE status=? AND next_retry_at<=? ORDER BY created_at LIMIT ?`, outbox.StatusPending, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	items := []outbox.Item{}
	for rows.Next() {
		var i outbox.Item
		var next, created int64
		if err := rows.Scan(&i.ID, &i.EventType, &i.Destination, &i.Payload, &i.IdempotencyKey, &i.Status, &i.RetryCount, &next, &i.LastError, &created); err != nil {
			rows.Close()
			return nil, err
		}
		i.NextRetryAt = time.Unix(next, 0).UTC()
		i.CreatedAt = time.Unix(created, 0).UTC()
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, i := range items {
		res, err := tx.ExecContext(ctx, `UPDATE outbox_events SET status=?,updated_at=? WHERE id=? AND status=?`, outbox.StatusProcessing, now.Unix(), i.ID, outbox.StatusPending)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return nil, fmt.Errorf("outbox claim lost for %s", i.ID)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (d *DB) MarkOutboxDelivered(ctx context.Context, id string) error {
	_, err := d.ExecContext(ctx, `UPDATE outbox_events SET status=?,updated_at=? WHERE id=?`, outbox.StatusDelivered, time.Now().Unix(), id)
	return err
}
func (d *DB) RetryOutbox(ctx context.Context, id, reason string, next time.Time, dead bool) error {
	status := outbox.StatusPending
	if dead {
		status = outbox.StatusDeadLetter
	}
	_, err := d.ExecContext(ctx, `UPDATE outbox_events SET status=?,retry_count=retry_count+1,next_retry_at=?,last_error=?,updated_at=? WHERE id=?`, status, next.Unix(), reason, time.Now().Unix(), id)
	return err
}
