package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

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
	b, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("marshal market analysis: %w", err)
	}
	_, err = d.Exec(`INSERT INTO market_analyses(timestamp,btc_price,regime,action_permission,risk_level,falling_knife_risk,fomo_risk,payload_json) VALUES(?,?,?,?,?,?,?,?)`, a.Timestamp.Unix(), a.BTCPrice, a.MarketRegime, string(a.ActionPermission), string(a.RiskLevel), string(a.FallingKnifeRisk), string(a.FomoRisk), string(b))
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
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal accumulation plan: %w", err)
	}
	_, err = d.Exec(`INSERT INTO accumulation_plans(timestamp,state,action_permission,payload_json) VALUES(?,?,?,?)`, p.Timestamp.Unix(), string(p.State), string(p.ActionPermission), string(b))
	return err
}
