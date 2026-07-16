package storage

import "database/sql"

func (d *DB) SetHermesDemoted(demoted bool) error {
	value := "false"
	if demoted {
		value = "true"
	}
	_, err := d.Exec(`INSERT OR REPLACE INTO operator_settings(key, value) VALUES('hermes_demoted', ?)`, value)
	return err
}

func (d *DB) IsHermesDemoted() (bool, error) {
	var value string
	err := d.QueryRow(`SELECT value FROM operator_settings WHERE key='hermes_demoted'`).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	return value == "true", nil
}
