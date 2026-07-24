package storage

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

const dcaGlobalCapInitial = 20.0

type DCAExecutionState struct {
	GlobalCapPercent float64
	UpdatedAt        time.Time
}

// DCAExecutionState returns the persisted global cap. It defaults to 20% and
// does not create execution authority or change the operator halt.
func (d *DB) DCAExecutionState() (DCAExecutionState, error) {
	var pct float64
	var updated int64
	err := d.QueryRow(`SELECT global_cap_percent,updated_at FROM dca_execution_state WHERE singleton=1`).Scan(&pct, &updated)
	if err == sql.ErrNoRows {
		return DCAExecutionState{GlobalCapPercent: dcaGlobalCapInitial}, nil
	}
	if err != nil {
		return DCAExecutionState{}, err
	}
	if pct < dcaGlobalCapInitial || pct > 100 || math.IsNaN(pct) || math.IsInf(pct, 0) {
		return DCAExecutionState{}, fmt.Errorf("DCA global cap invalid")
	}
	return DCAExecutionState{GlobalCapPercent: pct, UpdatedAt: time.Unix(updated, 0).UTC()}, nil
}

func advanceDCAExposureCapTx(tx *sql.Tx, clientOrderID string, now time.Time) error {
	var pct float64
	err := tx.QueryRow(`SELECT global_cap_percent FROM dca_execution_state WHERE singleton=1`).Scan(&pct)
	if err == sql.ErrNoRows {
		pct = dcaGlobalCapInitial
	} else if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRow(`SELECT client_order_id FROM dca_global_cap_events WHERE client_order_id=?`, clientOrderID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	next := math.Min(100, pct+20)
	if _, err = tx.Exec(`INSERT INTO dca_global_cap_events(client_order_id,previous_cap_percent,next_cap_percent,created_at) VALUES(?,?,?,?)`, clientOrderID, pct, next, now.Unix()); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO dca_execution_state(singleton,global_cap_percent,updated_at) VALUES(1,?,?) ON CONFLICT(singleton) DO UPDATE SET global_cap_percent=excluded.global_cap_percent,updated_at=excluded.updated_at`, next, now.Unix())
	return err
}

// AdvanceDCAExposureCapAfterReconciledFill advances by one 20% rung only for
// a unique confirmed FILLED order lifecycle. EXPIRED/CANCELLED/REJECTED cannot
// call this method. Callers must reconcile exchange state before invoking it.
func (d *DB) AdvanceDCAExposureCapAfterReconciledFill(clientOrderID string, now time.Time) (DCAExecutionState, bool, error) {
	clientOrderID = strings.TrimSpace(clientOrderID)
	if clientOrderID == "" {
		return DCAExecutionState{}, false, fmt.Errorf("client order ID required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := d.Begin()
	if err != nil {
		return DCAExecutionState{}, false, err
	}
	defer tx.Rollback()
	var pct float64
	var updated int64
	err = tx.QueryRow(`SELECT global_cap_percent,updated_at FROM dca_execution_state WHERE singleton=1`).Scan(&pct, &updated)
	if err == sql.ErrNoRows {
		pct = dcaGlobalCapInitial
	} else if err != nil {
		return DCAExecutionState{}, false, err
	}
	var existing string
	err = tx.QueryRow(`SELECT client_order_id FROM dca_global_cap_events WHERE client_order_id=?`, clientOrderID).Scan(&existing)
	if err == nil {
		if err = tx.Commit(); err != nil {
			return DCAExecutionState{}, false, err
		}
		return DCAExecutionState{GlobalCapPercent: pct, UpdatedAt: time.Unix(updated, 0).UTC()}, false, nil
	}
	if err != sql.ErrNoRows {
		return DCAExecutionState{}, false, err
	}
	next := math.Min(100, pct+20)
	if err = advanceDCAExposureCapTx(tx, clientOrderID, now); err != nil {
		return DCAExecutionState{}, false, err
	}
	next = math.Min(100, pct+20)
	if err = tx.Commit(); err != nil {
		return DCAExecutionState{}, false, err
	}
	return DCAExecutionState{GlobalCapPercent: next, UpdatedAt: now.UTC()}, true, nil
}
