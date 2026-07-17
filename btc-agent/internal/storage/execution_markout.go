package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// UpdateExecutionMarkouts measures owned fills only after each horizon has
// elapsed. BUY positive means price improved after fill; SELL positive means
// price declined after fill. Existing measurements are immutable/idempotent.
func (d *DB) UpdateExecutionMarkouts(now time.Time) (int, error) {
	type candidate struct {
		id, ts    int64
		sym, side string
		fill      float64
	}
	rows, err := d.Query(`SELECT e.id,e.timestamp,UPPER(e.symbol),UPPER(e.side),e.fill_price FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' ORDER BY e.id`)
	if err != nil {
		return 0, err
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.ts, &c.sym, &c.side, &c.fill); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	// DB intentionally has one connection. Close the source cursor before any
	// nested QueryRow/INSERT to avoid starving that connection forever.
	count := 0
	for _, c := range candidates {
		if c.fill <= 0 {
			continue
		}
		for _, h := range []int{1, 5, 15, 60} {
			target := time.Unix(c.ts, 0).Add(time.Duration(h) * time.Minute)
			if now.Before(target) {
				continue
			}
			var exists int
			if e := d.QueryRow(`SELECT COUNT(*) FROM execution_markouts WHERE event_id=? AND horizon_minutes=?`, c.id, h).Scan(&exists); e != nil {
				return count, e
			}
			if exists > 0 {
				continue
			}
			var mark float64
			e := d.QueryRow(`SELECT close FROM candles WHERE symbol=? AND interval='1m' AND close_time>=? ORDER BY close_time LIMIT 1`, c.sym, target.Unix()).Scan(&mark)
			if e != nil && e != sql.ErrNoRows {
				return count, e
			}
			if mark <= 0 {
				continue
			}
			m := (mark - c.fill) / c.fill
			if strings.EqualFold(c.side, "SELL") {
				m = -m
			}
			if e = d.SaveFillMarkout(FillMarkout{EventID: c.id, HorizonMinutes: h, MarkPrice: mark, MarkoutPct: m, MeasuredAt: now}); e != nil {
				return count, e
			}
			count++
		}
	}
	return count, nil
}
func (d *DB) ExecutionMarkoutSummary() string {
	var n int
	var avg float64
	_ = d.QueryRow(`SELECT COUNT(*),COALESCE(AVG(markout_pct),0) FROM execution_markouts`).Scan(&n, &avg)
	return fmt.Sprintf("markouts=%d avg=%.3f%%", n, avg*100)
}
