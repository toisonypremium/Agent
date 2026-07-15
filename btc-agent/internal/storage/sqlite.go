package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/microstructure"
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
		`CREATE TABLE IF NOT EXISTS performance_reviews(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_orders(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, type TEXT, price REAL, quantity REAL, notional REAL, status TEXT, submitted_at INTEGER, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_order_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_fills(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, filled_quantity REAL, avg_price REAL, fee REAL, fee_currency TEXT, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_positions(symbol TEXT PRIMARY KEY, inst_id TEXT, quantity REAL, avg_entry_price REAL, cost_basis REAL, fee_total REAL, fee_currency TEXT, updated_at INTEGER, opened_at INTEGER NOT NULL DEFAULT 0, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_position_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, delta_quantity REAL, fill_price REAL, notional_delta REAL, fee_delta REAL, fee_currency TEXT, position_qty REAL, avg_entry_price REAL, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS operator_settings(key TEXT PRIMARY KEY, value TEXT);`,
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
	} {
		if err := d.ensureColumn("live_orders", col.name, col.def); err != nil {
			return err
		}
	}
	if err := d.ensureColumn("live_positions", "opened_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
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
func (d *DB) SaveCandles(cs []market.Candle) error {
	return withSQLiteRetry(func() error {
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		st, err := tx.Prepare(`INSERT OR REPLACE INTO candles(symbol,interval,open_time,open,high,low,close,volume,close_time) VALUES(?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, c := range cs {
			if _, err := st.Exec(c.Symbol, c.Interval, c.OpenTime.Unix(), c.Open, c.High, c.Low, c.Close, c.Volume, c.CloseTime.Unix()); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (d *DB) LatestCandleOpenTime(symbol, interval string) (time.Time, bool, error) {
	var ts int64
	err := d.QueryRow(`SELECT open_time FROM candles WHERE symbol=? AND interval=? ORDER BY open_time DESC LIMIT 1`, strings.ToUpper(symbol), interval).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(ts, 0), true, nil
}

func (d *DB) CandleCount(symbol, interval string) (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM candles WHERE symbol=? AND interval=?`, strings.ToUpper(symbol), interval).Scan(&count)
	return count, err
}

func (d *DB) LoadCandles(symbol, interval string, limit int) ([]market.Candle, error) {
	rows, err := d.Query(`SELECT symbol,interval,open_time,open,high,low,close,volume,close_time FROM candles WHERE symbol=? AND interval=? ORDER BY open_time DESC LIMIT ?`, symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rev := []market.Candle{}
	for rows.Next() {
		var c market.Candle
		var ot, ct int64
		if err := rows.Scan(&c.Symbol, &c.Interval, &ot, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &ct); err != nil {
			return nil, err
		}
		c.OpenTime = time.Unix(ot, 0)
		c.CloseTime = time.Unix(ct, 0)
		rev = append(rev, c)
	}
	out := make([]market.Candle, len(rev))
	for i := range rev {
		out[len(rev)-1-i] = rev[i]
	}
	return out, rows.Err()
}
func (d *DB) SaveAnalysis(a agent1.MarketAnalysis) error {
	b, _ := json.Marshal(a)
	_, err := d.Exec(`INSERT INTO market_analyses(timestamp,btc_price,regime,action_permission,risk_level,falling_knife_risk,fomo_risk,payload_json) VALUES(?,?,?,?,?,?,?,?)`, a.Timestamp.Unix(), a.BTCPrice, a.MarketRegime, string(a.ActionPermission), string(a.RiskLevel), string(a.FallingKnifeRisk), string(a.FomoRisk), string(b))
	return err
}
func (d *DB) LatestAnalysis() (agent1.MarketAnalysis, error) {
	var js string
	err := d.QueryRow(`SELECT payload_json FROM market_analyses ORDER BY id DESC LIMIT 1`).Scan(&js)
	if err != nil {
		return agent1.MarketAnalysis{}, err
	}
	var a agent1.MarketAnalysis
	return a, json.Unmarshal([]byte(js), &a)
}

func (d *DB) LatestPlan() (agent2.Plan, error) {
	var js string
	err := d.QueryRow(`SELECT payload_json FROM accumulation_plans ORDER BY id DESC LIMIT 1`).Scan(&js)
	if err != nil {
		return agent2.Plan{}, err
	}
	var p agent2.Plan
	return p, json.Unmarshal([]byte(js), &p)
}

func (d *DB) SavePlan(p agent2.Plan) error {
	b, _ := json.Marshal(p)
	_, err := d.Exec(`INSERT INTO accumulation_plans(timestamp,state,action_permission,payload_json) VALUES(?,?,?,?)`, p.Timestamp.Unix(), string(p.State), string(p.ActionPermission), string(b))
	return err
}
func (d *DB) SaveOrders(orders []agent2.PaperOrder) error {
	for _, o := range orders {
		var existingStatus string
		err := d.QueryRow(`SELECT status FROM paper_orders WHERE id=?`, o.ID).Scan(&existingStatus)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && existingStatus != "OPEN" {
			continue
		}
		if err == sql.ErrNoRows && strings.EqualFold(o.Status, "OPEN") {
			exists, err := d.HasEquivalentOpenPaperOrder(o.Symbol, o.Layer, o.Price)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
		}
		_, err = d.Exec(`INSERT OR REPLACE INTO paper_orders(id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, o.ID, o.Timestamp.Unix(), o.Symbol, o.Side, o.Layer, o.Price, o.Quantity, o.Notional, o.Status, o.ExpiresAt.Unix(), o.InvalidationPrice, o.Reason)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) HasEquivalentOpenPaperOrder(symbol string, layer int, price float64) (bool, error) {
	rows, err := d.Query(`SELECT price FROM paper_orders WHERE status='OPEN' AND UPPER(symbol)=? AND layer=?`, strings.ToUpper(symbol), layer)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var existingPrice float64
		if err := rows.Scan(&existingPrice); err != nil {
			return false, err
		}
		if absFloat(existingPrice-price) <= 1e-9 {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (d *DB) UpdatePaperOrderStatus(id, status, reason string) error {
	_, err := d.Exec(`UPDATE paper_orders SET status=?, reason=? WHERE id=?`, status, reason, id)
	return err
}

func (d *DB) PaperOrderStatusCounts() (map[string]int, error) {
	rows, err := d.Query(`SELECT status, COUNT(*) FROM paper_orders GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (d *DB) OpenPaperOrders() ([]agent2.PaperOrder, error) {
	rows, err := d.Query(`SELECT id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason FROM paper_orders WHERE status='OPEN' ORDER BY timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := []agent2.PaperOrder{}
	for rows.Next() {
		var o agent2.PaperOrder
		var ts, exp int64
		if err := rows.Scan(&o.ID, &ts, &o.Symbol, &o.Side, &o.Layer, &o.Price, &o.Quantity, &o.Notional, &o.Status, &exp, &o.InvalidationPrice, &o.Reason); err != nil {
			return nil, err
		}
		o.Timestamp = time.Unix(ts, 0)
		o.ExpiresAt = time.Unix(exp, 0)
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (d *DB) SaveReport(t, content string) error {
	_, err := d.Exec(`INSERT INTO reports(timestamp,type,content) VALUES(?,?,?)`, time.Now().Unix(), t, content)
	return err
}

func (d *DB) SaveRuntimeEvent(e RuntimeEvent) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Source == "" {
		e.Source = "unknown"
	}
	if e.Type == "" {
		e.Type = "event"
	}
	if e.Severity == "" {
		e.Severity = "info"
	}
	if e.Fingerprint != "" {
		var existing int
		err := d.QueryRow(`SELECT id FROM runtime_events WHERE source=? AND type=? AND fingerprint=? ORDER BY id DESC LIMIT 1`, e.Source, e.Type, e.Fingerprint).Scan(&existing)
		if err == nil {
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
	}
	_, err := d.Exec(`INSERT INTO runtime_events(timestamp, source, type, severity, fingerprint, payload_json, handled_at) VALUES(?,?,?,?,?,?,NULL)`, e.Timestamp.Unix(), e.Source, e.Type, e.Severity, e.Fingerprint, e.PayloadJSON)
	return err
}

func (d *DB) PendingRuntimeEvents(limit int) ([]RuntimeEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`SELECT id, timestamp, source, type, severity, fingerprint, payload_json, handled_at FROM runtime_events WHERE handled_at IS NULL OR handled_at=0 ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuntimeEvent{}
	for rows.Next() {
		var e RuntimeEvent
		var ts int64
		var handled sql.NullInt64
		if err := rows.Scan(&e.ID, &ts, &e.Source, &e.Type, &e.Severity, &e.Fingerprint, &e.PayloadJSON, &handled); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		if handled.Valid && handled.Int64 > 0 {
			e.HandledAt = time.Unix(handled.Int64, 0)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) MarkRuntimeEventHandled(id int64, handledAt time.Time) error {
	if id <= 0 {
		return fmt.Errorf("runtime event id required")
	}
	if handledAt.IsZero() {
		handledAt = time.Now().UTC()
	}
	_, err := d.Exec(`UPDATE runtime_events SET handled_at=? WHERE id=?`, handledAt.Unix(), id)
	return err
}

func (d *DB) SaveMicrostructureSnapshot(s microstructure.Snapshot) error {
	if s.Timestamp.IsZero() {
		s.Timestamp = time.Now().UTC()
	}
	if s.Source == "" {
		s.Source = "unknown"
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT INTO microstructure_snapshots(timestamp, symbol, source, status, fingerprint, payload_json) VALUES(?,?,?,?,?,?)`, s.Timestamp.Unix(), strings.ToUpper(s.Symbol), s.Source, microstructureSnapshotStatus(s), microstructureSnapshotFingerprint(s), string(b))
	return err
}

func (d *DB) SaveMicrostructureSnapshots(items []microstructure.Snapshot) error {
	return withSQLiteRetry(func() error {
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		st, err := tx.Prepare(`INSERT INTO microstructure_snapshots(timestamp, symbol, source, status, fingerprint, payload_json) VALUES(?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, s := range items {
			if s.Timestamp.IsZero() {
				s.Timestamp = time.Now().UTC()
			}
			if s.Source == "" {
				s.Source = "unknown"
			}
			b, err := json.Marshal(s)
			if err != nil {
				return err
			}
			if _, err := st.Exec(s.Timestamp.Unix(), strings.ToUpper(s.Symbol), s.Source, microstructureSnapshotStatus(s), microstructureSnapshotFingerprint(s), string(b)); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (d *DB) LatestMicrostructureSnapshots(symbols []string) ([]microstructure.Snapshot, error) {
	out := []microstructure.Snapshot{}
	for _, symbol := range symbols {
		rows, err := d.LoadMicrostructureSnapshots(symbol, 1)
		if err != nil {
			return out, err
		}
		if len(rows) > 0 {
			out = append(out, rows[0])
		}
	}
	return out, nil
}

func (d *DB) LoadMicrostructureSnapshots(symbol string, limit int) ([]microstructure.Snapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`SELECT payload_json FROM microstructure_snapshots WHERE symbol=? ORDER BY timestamp DESC, id DESC LIMIT ?`, strings.ToUpper(symbol), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []microstructure.Snapshot{}
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		var s microstructure.Snapshot
		if err := json.Unmarshal([]byte(js), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func microstructureSnapshotStatus(s microstructure.Snapshot) string {
	if s.Health.Fresh {
		return microstructure.StatusOK
	}
	if len(s.Health.Blockers) > 0 {
		return microstructure.StatusBlock
	}
	return microstructure.StatusWarn
}

func microstructureSnapshotFingerprint(s microstructure.Snapshot) string {
	return fmt.Sprintf("%s|fresh=%v|buy=%s|cvd=%s|ob=%s|fund=%s|basis=%s|b=%d|w=%d", strings.ToUpper(s.Symbol), s.Health.Fresh, s.Signals.BuyPressure, s.Signals.CVDTrend, s.Signals.OrderBookBias, s.Signals.FundingBias, s.Signals.BasisBias, len(s.Health.Blockers), len(s.Health.Warnings))
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

func (d *DB) HasManagedRealOrderSubmission() (bool, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM live_orders WHERE status IN ('SUBMITTED', 'PARTIAL_FILL', 'LIVE_OPEN', 'PARTIALLY_FILLED', 'FILLED') AND (source LIKE 'deterministic_agent2_layer_%' OR last_management_action LIKE 'submitted%' OR last_management_action LIKE 'placed:%')`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
		if event.Timestamp > 0 {
			pos.OpenedAt = event.Timestamp
		} else {
			pos.OpenedAt = time.Now().Unix()
		}
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
	_, err = tx.Exec(`INSERT OR REPLACE INTO live_positions(symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at, payload_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, pos.Symbol, pos.InstID, pos.Quantity, pos.AvgEntryPrice, pos.CostBasis, pos.FeeTotal, pos.FeeCurrency, pos.UpdatedAt, pos.OpenedAt, string(b))
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
	err := q.QueryRow(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at FROM live_positions WHERE symbol=?`, symbol).Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt, &pos.OpenedAt)
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
	rows, err := d.Query(`SELECT symbol, inst_id, quantity, avg_entry_price, cost_basis, fee_total, fee_currency, updated_at, opened_at FROM live_positions ORDER BY symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.LivePosition{}
	for rows.Next() {
		var pos live.LivePosition
		if err := rows.Scan(&pos.Symbol, &pos.InstID, &pos.Quantity, &pos.AvgEntryPrice, &pos.CostBasis, &pos.FeeTotal, &pos.FeeCurrency, &pos.UpdatedAt, &pos.OpenedAt); err != nil {
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
