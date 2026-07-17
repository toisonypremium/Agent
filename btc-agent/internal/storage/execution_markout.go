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
	rows, err := d.Query(`SELECT e.id,e.timestamp,UPPER(e.symbol),UPPER(e.side),e.fill_price FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' ORDER BY e.id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, ts int64
		var sym, side string
		var fill float64
		if err = rows.Scan(&id, &ts, &sym, &side, &fill); err != nil {
			return count, err
		}
		if fill <= 0 {
			continue
		}
		for _, h := range []int{1, 5, 15, 60} {
			target := time.Unix(ts, 0).Add(time.Duration(h) * time.Minute)
			if now.Before(target) {
				continue
			}
			var exists int
			if e := d.QueryRow(`SELECT COUNT(*) FROM execution_markouts WHERE event_id=? AND horizon_minutes=?`, id, h).Scan(&exists); e != nil {
				return count, e
			}
			if exists > 0 {
				continue
			}
			mark := 0.0
			e := d.QueryRow(`SELECT close FROM candles WHERE symbol=? AND interval='1m' AND close_time>=? ORDER BY close_time LIMIT 1`, sym, target.Unix()).Scan(&mark)
			if e != nil && e != sql.ErrNoRows {
				return count, e
			}
			if mark <= 0 {
				continue
			}
			m := (mark - fill) / fill
			if strings.EqualFold(side, "SELL") {
				m = -m
			}
			if e = d.SaveFillMarkout(FillMarkout{EventID: id, HorizonMinutes: h, MarkPrice: mark, MarkoutPct: m, MeasuredAt: now}); e != nil {
				return count, e
			}
			count++
		}
	}
	if err = rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}
func (d *DB) ExecutionMarkoutSummary() string {
	var n int
	var avg float64
	_ = d.QueryRow(`SELECT COUNT(*),COALESCE(AVG(markout_pct),0) FROM execution_markouts`).Scan(&n, &avg)
	return fmt.Sprintf("markouts=%d avg=%.3f%%", n, avg*100)
}
