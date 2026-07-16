package storage

import (
	"database/sql"
	"strings"
	"time"

	"btc-agent/internal/agent2"
)

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
