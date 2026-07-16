package storage

import (
	"database/sql"
	"strings"
	"time"
)

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
