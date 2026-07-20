package storage

import "time"

// RecentRuntimeEvents is a read-only dashboard query. It never changes handled_at.
func (d *DB) RecentRuntimeEvents(limit int, beforeID int64) ([]RuntimeEvent, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	query := `SELECT id,timestamp,source,type,severity,fingerprint,payload_json,handled_at FROM runtime_events`
	args := []any{}
	if beforeID > 0 {
		query += ` WHERE id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuntimeEvent{}
	for rows.Next() {
		var e RuntimeEvent
		var ts int64
		var handled any
		if err := rows.Scan(&e.ID, &ts, &e.Source, &e.Type, &e.Severity, &e.Fingerprint, &e.PayloadJSON, &handled); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}
