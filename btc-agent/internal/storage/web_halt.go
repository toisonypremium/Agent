package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type WebHaltReceipt struct {
	RequestID string    `json:"request_id"`
	Identity  string    `json:"identity"`
	HaltedAt  time.Time `json:"halted_at"`
	Duplicate bool      `json:"duplicate"`
}

// RequestWebHalt is intentionally one-way: it can only set halted=true. The
// idempotency key is hashed before persistence and every accepted request is
// recorded in both the runtime and operator audit ledgers in one transaction.
func (d *DB) RequestWebHalt(identity, reason, idempotencyKey string, now time.Time) (WebHaltReceipt, error) {
	identity = strings.TrimSpace(strings.ToLower(identity))
	reason = strings.TrimSpace(reason)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if identity == "" || len(identity) > 320 {
		return WebHaltReceipt{}, fmt.Errorf("identity invalid")
	}
	if len(reason) < 8 || len(reason) > 500 {
		return WebHaltReceipt{}, fmt.Errorf("reason length invalid")
	}
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 128 {
		return WebHaltReceipt{}, fmt.Errorf("idempotency key invalid")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	hashBytes := sha256.Sum256([]byte(idempotencyKey))
	requestID := hex.EncodeToString(hashBytes[:])
	tx, err := d.Begin()
	if err != nil {
		return WebHaltReceipt{}, err
	}
	defer tx.Rollback()
	var existingIdentity string
	var createdAt int64
	err = tx.QueryRow(`SELECT identity,created_at FROM web_halt_requests WHERE idempotency_hash=?`, requestID).Scan(&existingIdentity, &createdAt)
	if err == nil {
		if existingIdentity != identity {
			return WebHaltReceipt{}, fmt.Errorf("idempotency key belongs to another identity")
		}
		return WebHaltReceipt{RequestID: requestID, Identity: identity, HaltedAt: time.Unix(createdAt, 0).UTC(), Duplicate: true}, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return WebHaltReceipt{}, err
	}
	payload, _ := json.Marshal(map[string]string{"request_id": requestID, "reason": reason})
	if _, err = tx.Exec(`INSERT INTO web_halt_requests(idempotency_hash,identity,reason,created_at) VALUES(?,?,?,?)`, requestID, identity, reason, now.Unix()); err != nil {
		return WebHaltReceipt{}, err
	}
	if _, err = tx.Exec(`INSERT INTO runtime_events(timestamp,source,type,severity,fingerprint,payload_json,handled_at) VALUES(?,?,?,?,?,?,NULL)`, now.Unix(), "web-console", "operator_halt_request", "critical", "web-halt:"+requestID, string(payload)); err != nil {
		return WebHaltReceipt{}, err
	}
	if _, err = tx.Exec(`INSERT INTO operator_audit_events(timestamp,identity,action,result,request_id,payload_json) VALUES(?,?,?,?,?,?)`, now.Unix(), identity, "HALT", "APPLIED", requestID, string(payload)); err != nil {
		return WebHaltReceipt{}, err
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO operator_settings(key,value) VALUES('halted','true')`); err != nil {
		return WebHaltReceipt{}, err
	}
	if err = tx.Commit(); err != nil {
		return WebHaltReceipt{}, err
	}
	return WebHaltReceipt{RequestID: requestID, Identity: identity, HaltedAt: now.UTC()}, nil
}

// EnsureWebHaltSchema adds only the idempotency ledger required by the narrow
// web halt bridge. It does not run the wider runtime migration set.
func (d *DB) EnsureWebHaltSchema() error {
	_, err := d.Exec(`CREATE TABLE IF NOT EXISTS web_halt_requests(idempotency_hash TEXT PRIMARY KEY, identity TEXT NOT NULL, reason TEXT NOT NULL, created_at INTEGER NOT NULL)`)
	return err
}
