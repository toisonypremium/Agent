package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const (
	LLMReservationInFlight = "IN_FLIGHT"
	LLMReservationSuccess  = "SUCCESS"
	LLMReservationError    = "ERROR"
)

type LLMCallReservation struct {
	Purpose     string    `json:"purpose"`
	StateHash   string    `json:"state_hash"`
	Status      string    `json:"status"`
	ReservedAt  time.Time `json:"reserved_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
	ValidUntil  time.Time `json:"valid_until,omitempty"`
	ErrorClass  string    `json:"error_class,omitempty"`
}

func (d *DB) ReserveLLMCall(ctx context.Context, purpose, stateHash string, now time.Time, inFlightTTL time.Duration) (LLMCallReservation, bool, string, error) {
	if purpose == "" || stateHash == "" {
		return LLMCallReservation{}, false, "INVALID_RESERVATION", fmt.Errorf("purpose and state hash required")
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return LLMCallReservation{}, false, "STORAGE_ERROR", err
	}
	defer tx.Rollback()
	current, found, err := loadLLMReservationTx(ctx, tx, purpose, stateHash)
	if err != nil {
		return current, false, "STORAGE_ERROR", err
	}
	if found {
		switch current.Status {
		case LLMReservationSuccess:
			if current.ValidUntil.After(now) {
				return current, false, "DECISION_STILL_FRESH", nil
			}
		case LLMReservationError:
			if current.NextRetryAt.After(now) {
				return current, false, "ERROR_COOLDOWN", nil
			}
		case LLMReservationInFlight:
			if current.ReservedAt.Add(inFlightTTL).After(now) {
				return current, false, "STATE_IN_FLIGHT", nil
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO llm_call_reservations(purpose,state_hash,status,reserved_at,finished_at,next_retry_at,valid_until,error_class) VALUES(?,?,?, ?,0,0,0,'') ON CONFLICT(purpose,state_hash) DO UPDATE SET status=excluded.status,reserved_at=excluded.reserved_at,finished_at=0,next_retry_at=0,valid_until=0,error_class=''`, purpose, stateHash, LLMReservationInFlight, now.Unix())
	if err != nil {
		return current, false, "STORAGE_ERROR", err
	}
	if err = tx.Commit(); err != nil {
		return current, false, "STORAGE_ERROR", err
	}
	return LLMCallReservation{Purpose: purpose, StateHash: stateHash, Status: LLMReservationInFlight, ReservedAt: now}, true, "", nil
}

func (d *DB) CompleteLLMCall(ctx context.Context, purpose, stateHash, status, errorClass string, now, nextRetryAt, validUntil time.Time) error {
	if status != LLMReservationSuccess && status != LLMReservationError {
		return fmt.Errorf("invalid LLM reservation terminal status %q", status)
	}
	res, err := d.ExecContext(ctx, `UPDATE llm_call_reservations SET status=?,finished_at=?,next_retry_at=?,valid_until=?,error_class=? WHERE purpose=? AND state_hash=? AND status=?`, status, now.Unix(), unixOrZero(nextRetryAt), unixOrZero(validUntil), errorClass, purpose, stateHash, LLMReservationInFlight)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("LLM reservation completion lost")
	}
	return nil
}

func (d *DB) LLMCallReservation(purpose, stateHash string) (LLMCallReservation, bool, error) {
	return loadLLMReservationQuery(d.DB, purpose, stateHash)
}

type llmReservationQuery interface {
	QueryRow(string, ...any) *sql.Row
}

func loadLLMReservationQuery(q llmReservationQuery, purpose, stateHash string) (LLMCallReservation, bool, error) {
	var r LLMCallReservation
	var reserved, finished, retry, valid int64
	err := q.QueryRow(`SELECT purpose,state_hash,status,reserved_at,finished_at,next_retry_at,valid_until,error_class FROM llm_call_reservations WHERE purpose=? AND state_hash=?`, purpose, stateHash).Scan(&r.Purpose, &r.StateHash, &r.Status, &reserved, &finished, &retry, &valid, &r.ErrorClass)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.ReservedAt = time.Unix(reserved, 0).UTC()
	r.FinishedAt = timeOrZero(finished)
	r.NextRetryAt = timeOrZero(retry)
	r.ValidUntil = timeOrZero(valid)
	return r, true, nil
}

func loadLLMReservationTx(ctx context.Context, tx *sql.Tx, purpose, stateHash string) (LLMCallReservation, bool, error) {
	var r LLMCallReservation
	var reserved, finished, retry, valid int64
	err := tx.QueryRowContext(ctx, `SELECT purpose,state_hash,status,reserved_at,finished_at,next_retry_at,valid_until,error_class FROM llm_call_reservations WHERE purpose=? AND state_hash=?`, purpose, stateHash).Scan(&r.Purpose, &r.StateHash, &r.Status, &reserved, &finished, &retry, &valid, &r.ErrorClass)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.ReservedAt = time.Unix(reserved, 0).UTC()
	r.FinishedAt = timeOrZero(finished)
	r.NextRetryAt = timeOrZero(retry)
	r.ValidUntil = timeOrZero(valid)
	return r, true, nil
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func timeOrZero(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}
