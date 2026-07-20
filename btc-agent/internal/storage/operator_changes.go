package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/operatorchange"
)

func (d *DB) SaveOperatorChange(r operatorchange.Request) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = d.Exec(`INSERT INTO operator_change_requests(id,action,requester,status,created_at,expires_at,payload_json) VALUES(?,?,?,?,?,?,?)`, r.ID, string(r.Action), strings.ToLower(r.Requester), string(r.Status), r.CreatedAt.Unix(), r.ExpiresAt.Unix(), string(b))
	return e
}
func (d *DB) OperatorChange(id string) (operatorchange.Request, error) {
	var raw string
	if e := d.QueryRow(`SELECT payload_json FROM operator_change_requests WHERE id=?`, id).Scan(&raw); e != nil {
		return operatorchange.Request{}, e
	}
	var r operatorchange.Request
	e := json.Unmarshal([]byte(raw), &r)
	return r, e
}
func (d *DB) ConfirmOperatorChange(id, identity string, at time.Time) (operatorchange.Request, error) {
	tx, e := d.Begin()
	if e != nil {
		return operatorchange.Request{}, e
	}
	defer tx.Rollback()
	var raw string
	if e = tx.QueryRow(`SELECT payload_json FROM operator_change_requests WHERE id=?`, id).Scan(&raw); e != nil {
		return operatorchange.Request{}, e
	}
	var r operatorchange.Request
	if e = json.Unmarshal([]byte(raw), &r); e != nil {
		return r, e
	}
	if r.Status != operatorchange.Pending {
		return r, errors.New("change is not pending")
	}
	r, e = operatorchange.Confirm(r, identity)
	if e != nil {
		return r, e
	}
	b, marshalErr := json.Marshal(r)
	if marshalErr != nil {
		return r, fmt.Errorf("marshal operator change confirmation: %w", marshalErr)
	}
	if _, e = tx.Exec(`INSERT INTO operator_change_confirmations(request_id,identity,confirmed_at) VALUES(?,?,?)`, id, strings.ToLower(identity), at.Unix()); e != nil {
		return r, e
	}
	if _, e = tx.Exec(`UPDATE operator_change_requests SET status=?,payload_json=? WHERE id=?`, string(r.Status), string(b), id); e != nil {
		return r, e
	}
	return r, tx.Commit()
}
