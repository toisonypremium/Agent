package storage

import (
	"database/sql"
	"fmt"
	"time"
)

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
