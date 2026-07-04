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
	"btc-agent/internal/market"
	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Disable mmap for proot compatibility (avoids "out of memory" on Android/proot)
	dsn := "file:" + path + "?_pragma=mmap_size(0)&_pragma=journal_mode(wal)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
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
		`CREATE TABLE IF NOT EXISTS performance_reviews(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_orders(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, type TEXT, price REAL, quantity REAL, notional REAL, status TEXT, submitted_at INTEGER, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_order_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_fills(client_order_id TEXT PRIMARY KEY, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, filled_quantity REAL, avg_price REAL, fee REAL, fee_currency TEXT, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_positions(symbol TEXT PRIMARY KEY, inst_id TEXT, quantity REAL, avg_entry_price REAL, cost_basis REAL, fee_total REAL, fee_currency TEXT, updated_at INTEGER, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS live_position_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, inst_id TEXT, symbol TEXT, side TEXT, delta_quantity REAL, fill_price REAL, notional_delta REAL, fee_delta REAL, fee_currency TEXT, position_qty REAL, avg_entry_price REAL, status TEXT, payload_json TEXT);`,
		`CREATE TABLE IF NOT EXISTS operator_settings(key TEXT PRIMARY KEY, value TEXT);`,
		`INSERT OR IGNORE INTO operator_settings(key, value) VALUES('halted', 'true');`}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
func (d *DB) SaveCandles(cs []market.Candle) error {
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
		_, err := d.Exec(`INSERT OR REPLACE INTO paper_orders(id,timestamp,symbol,side,layer,price,quantity,notional,status,expires_at,invalidation_price,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, o.ID, o.Timestamp.Unix(), o.Symbol, o.Side, o.Layer, o.Price, o.Quantity, o.Notional, o.Status, o.ExpiresAt.Unix(), o.InvalidationPrice, o.Reason)
		if err != nil {
			return err
		}
	}
	return nil
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

func (d *DB) SaveLiveOrderFromParams(clientOrderID, orderID, instID, symbol, side, ordType string, price, quantity, notional float64, status string) error {
	now := time.Now().Unix()
	_, err := d.Exec(
		`INSERT OR REPLACE INTO live_orders(client_order_id, order_id, inst_id, symbol, side, type, price, quantity, notional, status, submitted_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		clientOrderID, orderID, instID, symbol, side, ordType, price, quantity, notional, status, now, now,
	)
	return err
}

func (d *DB) OpenLiveOrders() ([]live.OrderStatus, error) {
	rows, err := d.Query(`SELECT client_order_id, order_id, inst_id, type, side, price, quantity, status, updated_at FROM live_orders WHERE status IN ('LIVE_OPEN', 'PARTIALLY_FILLED', 'SUBMITTED')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []live.OrderStatus{}
	for rows.Next() {
		var o live.OrderStatus
		var uTime int64
		if err := rows.Scan(&o.ClientOrderID, &o.OrderID, &o.InstID, &o.OrderType, &o.Side, &o.Price, &o.Quantity, &o.Status, &uTime); err != nil {
			return nil, err
		}
		o.UpdatedAt = uTime
		out = append(out, o)
	}
	return out, rows.Err()
}

func (d *DB) SaveLiveOrderStatus(o live.OrderStatus) error {
	now := time.Now().Unix()
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
			o.Status, now, string(b), id,
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
