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

const (
	HermesReceiptReserved  = "RESERVED"
	HermesReceiptCompleted = "COMPLETED"
	HermesReceiptPartial   = "PARTIAL"
	HermesReceiptUnknown   = "UNKNOWN"
	HermesReceiptBlocked   = "BLOCKED"
)

type HermesExecutionReceipt struct {
	DecisionID  string    `json:"decision_id"`
	PayloadHash string    `json:"payload_hash"`
	Status      string    `json:"status"`
	ReservedAt  time.Time `json:"reserved_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Detail      string    `json:"detail,omitempty"`
}

func HermesDecisionPayloadHash(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// ReserveHermesExecution atomically consumes a decision ID before any
// production side effect. A receipt is never deleted: restart and concurrent
// processes therefore cannot execute the same decision twice.
func (d *DB) ReserveHermesExecution(decisionID, payloadHash string, now time.Time) error {
	decisionID, payloadHash = strings.TrimSpace(decisionID), strings.TrimSpace(payloadHash)
	if decisionID == "" || payloadHash == "" {
		return fmt.Errorf("hermes execution receipt requires decision ID and payload hash")
	}
	_, err := d.Exec(`INSERT INTO hermes_execution_receipts(decision_id,payload_hash,status,reserved_at,updated_at,detail) VALUES(?,?,?,?,?,?)`, decisionID, payloadHash, HermesReceiptReserved, now.Unix(), now.Unix(), "execution authority reserved")
	if err == nil {
		return nil
	}
	var existingHash, status string
	lookupErr := d.QueryRow(`SELECT payload_hash,status FROM hermes_execution_receipts WHERE decision_id=?`, decisionID).Scan(&existingHash, &status)
	if lookupErr != nil && lookupErr != sql.ErrNoRows {
		return fmt.Errorf("reserve Hermes execution receipt: %w", err)
	}
	if existingHash != "" && existingHash != payloadHash {
		return fmt.Errorf("hermes decision ID payload mismatch: decision_id=%s", decisionID)
	}
	return fmt.Errorf("hermes decision replay blocked: decision_id=%s status=%s", decisionID, status)
}

func (d *DB) CompleteHermesExecution(decisionID, status, detail string, now time.Time) error {
	switch status {
	case HermesReceiptCompleted, HermesReceiptPartial, HermesReceiptUnknown, HermesReceiptBlocked:
	default:
		return fmt.Errorf("invalid Hermes execution receipt status: %s", status)
	}
	result, err := d.Exec(`UPDATE hermes_execution_receipts SET status=?,updated_at=?,detail=? WHERE decision_id=?`, status, now.Unix(), detail, strings.TrimSpace(decisionID))
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("hermes execution receipt not found: %s", decisionID)
	}
	return nil
}

func (d *DB) HermesExecutionReceipt(decisionID string) (HermesExecutionReceipt, error) {
	var receipt HermesExecutionReceipt
	var reservedAt, updatedAt int64
	err := d.QueryRow(`SELECT decision_id,payload_hash,status,reserved_at,updated_at,detail FROM hermes_execution_receipts WHERE decision_id=?`, strings.TrimSpace(decisionID)).Scan(&receipt.DecisionID, &receipt.PayloadHash, &receipt.Status, &reservedAt, &updatedAt, &receipt.Detail)
	receipt.ReservedAt, receipt.UpdatedAt = time.Unix(reservedAt, 0).UTC(), time.Unix(updatedAt, 0).UTC()
	return receipt, err
}
