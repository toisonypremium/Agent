package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/microstructure"
)

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

// LoadMicrostructureHistory loads recent snapshots for each symbol to enable
// time-series analysis (e.g., MM footprint detection).
// Returns map[symbol][]Snapshot newest-first, up to limit per symbol.
func (d *DB) LoadMicrostructureHistory(symbols []string, limit int) (map[string][]microstructure.Snapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	out := make(map[string][]microstructure.Snapshot, len(symbols))
	for _, symbol := range symbols {
		rows, err := d.LoadMicrostructureSnapshots(strings.ToUpper(symbol), limit)
		if err != nil {
			return out, fmt.Errorf("load microstructure history %s: %w", symbol, err)
		}
		out[strings.ToUpper(symbol)] = rows
	}
	return out, nil
}
