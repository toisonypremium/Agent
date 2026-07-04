package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		`CREATE TABLE IF NOT EXISTS live_order_events(id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER, client_order_id TEXT, order_id TEXT, status TEXT, payload_json TEXT);`}
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
