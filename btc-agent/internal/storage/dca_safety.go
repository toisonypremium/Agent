package storage

import (
	"fmt"
	"strings"
	"time"
)

type DCASafetyState struct {
	ConsecutiveErrors, StaleObserverEpochs int
	AutoHaltReason                         string
	UpdatedAt                              time.Time
}

func (d *DB) DCASafetyState() (DCASafetyState, error) {
	var s DCASafetyState
	var at int64
	err := d.QueryRow(`SELECT consecutive_errors,stale_observer_epochs,auto_halt_reason,updated_at FROM dca_safety_state WHERE singleton=1`).Scan(&s.ConsecutiveErrors, &s.StaleObserverEpochs, &s.AutoHaltReason, &at)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return s, nil
		}
		return s, err
	}
	s.UpdatedAt = time.Unix(at, 0).UTC()
	return s, nil
}

// RecordDCASafetyCycle is scheduler-only safety accounting. A qualifying failure
// makes the existing operator halt one-way; it never resumes execution.
func (d *DB) RecordDCASafetyCycle(errCycle, staleArtifact bool, reason string, now time.Time) (DCASafetyState, bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, e := d.Begin()
	if e != nil {
		return DCASafetyState{}, false, e
	}
	defer tx.Rollback()
	var s DCASafetyState
	var at int64
	e = tx.QueryRow(`SELECT consecutive_errors,stale_observer_epochs,auto_halt_reason,updated_at FROM dca_safety_state WHERE singleton=1`).Scan(&s.ConsecutiveErrors, &s.StaleObserverEpochs, &s.AutoHaltReason, &at)
	if e != nil && !strings.Contains(e.Error(), "no rows") {
		return s, false, e
	}
	if errCycle {
		s.ConsecutiveErrors++
	} else {
		s.ConsecutiveErrors = 0
	}
	if staleArtifact {
		s.StaleObserverEpochs++
	} else {
		s.StaleObserverEpochs = 0
	}
	halt := s.ConsecutiveErrors >= 3 || s.StaleObserverEpochs >= 2
	if halt && s.AutoHaltReason == "" {
		s.AutoHaltReason = strings.TrimSpace(reason)
		if s.AutoHaltReason == "" {
			s.AutoHaltReason = "dca_safety_policy"
		}
	}
	if _, e = tx.Exec(`INSERT INTO dca_safety_state(singleton,consecutive_errors,stale_observer_epochs,auto_halt_reason,updated_at) VALUES(1,?,?,?,?) ON CONFLICT(singleton) DO UPDATE SET consecutive_errors=excluded.consecutive_errors,stale_observer_epochs=excluded.stale_observer_epochs,auto_halt_reason=CASE WHEN dca_safety_state.auto_halt_reason<>'' THEN dca_safety_state.auto_halt_reason ELSE excluded.auto_halt_reason END,updated_at=excluded.updated_at`, s.ConsecutiveErrors, s.StaleObserverEpochs, s.AutoHaltReason, now.Unix()); e != nil {
		return s, false, e
	}
	if halt {
		if _, e = tx.Exec(`INSERT OR REPLACE INTO operator_settings(key,value) VALUES('halted','true')`); e != nil {
			return s, false, e
		}
		if _, e = tx.Exec(`INSERT INTO runtime_events(timestamp,source,type,severity,fingerprint,payload_json,handled_at) VALUES(?,?,?,?,?,?,NULL)`, now.Unix(), "dca-safety", "dca_auto_halt", "critical", "dca-auto-halt:"+s.AutoHaltReason, fmt.Sprintf(`{"reason":%q}`, s.AutoHaltReason)); e != nil {
			return s, false, e
		}
	}
	if e = tx.Commit(); e != nil {
		return s, false, e
	}
	s.UpdatedAt = now.UTC()
	return s, halt, nil
}
