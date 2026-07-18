package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type ThesisPositionState string

const (
	ThesisPositionPlanned           ThesisPositionState = "PLANNED"
	ThesisPositionProbeOpen         ThesisPositionState = "PROBE_OPEN"
	ThesisPositionAccumulating      ThesisPositionState = "ACCUMULATING"
	ThesisPositionOpen              ThesisPositionState = "POSITION_OPEN"
	ThesisPositionTargetReview      ThesisPositionState = "TARGET_REVIEW"
	ThesisPositionProtected         ThesisPositionState = "PROTECTED"
	ThesisPositionInvalidatedReview ThesisPositionState = "INVALIDATED_REVIEW"
	ThesisPositionClosed            ThesisPositionState = "CLOSED"
	ThesisPositionManualReview      ThesisPositionState = "MANUAL_REVIEW"
)

type ThesisPositionLifecycle struct {
	ThesisID           string              `json:"thesis_id"`
	Symbol             string              `json:"symbol"`
	State              ThesisPositionState `json:"state"`
	InvalidationPrice  float64             `json:"invalidation_price,omitempty"`
	PrimaryTargetPrice float64             `json:"primary_target_price,omitempty"`
	ProtectionPrice    float64             `json:"protection_price,omitempty"`
	PositionQuantity   float64             `json:"position_quantity,omitempty"`
	AvgEntryPrice      float64             `json:"avg_entry_price,omitempty"`
	OpenedAt           time.Time           `json:"opened_at,omitempty"`
	LastEvaluatedAt    time.Time           `json:"last_evaluated_at,omitempty"`
	Version            int64               `json:"version"`
}

func ValidateThesisPositionLifecycle(v ThesisPositionLifecycle) error {
	if strings.TrimSpace(v.ThesisID) == "" {
		return fmt.Errorf("thesis lifecycle thesis_id required")
	}
	if strings.TrimSpace(v.Symbol) == "" {
		return fmt.Errorf("thesis lifecycle symbol required")
	}
	if !validThesisPositionState(v.State) {
		return fmt.Errorf("invalid thesis position state %q", v.State)
	}
	for name, value := range map[string]float64{"invalidation_price": v.InvalidationPrice, "primary_target_price": v.PrimaryTargetPrice, "protection_price": v.ProtectionPrice, "position_quantity": v.PositionQuantity, "avg_entry_price": v.AvgEntryPrice} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s must be finite and non-negative", name)
		}
	}
	if v.PositionQuantity > 0 && v.AvgEntryPrice <= 0 {
		return fmt.Errorf("positive lifecycle quantity requires avg_entry_price")
	}
	if v.Version < 0 {
		return fmt.Errorf("thesis lifecycle version must be non-negative")
	}
	return nil
}

func validThesisPositionState(s ThesisPositionState) bool {
	switch s {
	case ThesisPositionPlanned, ThesisPositionProbeOpen, ThesisPositionAccumulating, ThesisPositionOpen, ThesisPositionTargetReview, ThesisPositionProtected, ThesisPositionInvalidatedReview, ThesisPositionClosed, ThesisPositionManualReview:
		return true
	}
	return false
}
func ThesisPositionStateAllowsDCA(s ThesisPositionState) bool {
	return s == ThesisPositionPlanned || s == ThesisPositionProbeOpen || s == ThesisPositionAccumulating
}

func validThesisPositionTransition(from, to ThesisPositionState) bool {
	if from == to {
		return true
	}
	if from == ThesisPositionClosed {
		return false
	}
	switch to {
	case ThesisPositionInvalidatedReview, ThesisPositionManualReview, ThesisPositionClosed:
		return true
	}
	switch from {
	case ThesisPositionPlanned:
		return to == ThesisPositionProbeOpen || to == ThesisPositionAccumulating
	case ThesisPositionProbeOpen:
		return to == ThesisPositionAccumulating || to == ThesisPositionOpen
	case ThesisPositionAccumulating:
		return to == ThesisPositionOpen || to == ThesisPositionTargetReview || to == ThesisPositionProtected
	case ThesisPositionOpen:
		return to == ThesisPositionTargetReview || to == ThesisPositionProtected
	case ThesisPositionTargetReview:
		return to == ThesisPositionProtected || to == ThesisPositionOpen
	case ThesisPositionProtected:
		return to == ThesisPositionTargetReview || to == ThesisPositionOpen
	case ThesisPositionInvalidatedReview, ThesisPositionManualReview:
		return false
	}
	return false
}

func (d *DB) SaveThesisPositionLifecycle(v ThesisPositionLifecycle) error {
	v.ThesisID = strings.TrimSpace(v.ThesisID)
	v.Symbol = strings.ToUpper(strings.TrimSpace(v.Symbol))
	v.State = ThesisPositionState(strings.ToUpper(strings.TrimSpace(string(v.State))))
	if v.Version == 0 {
		v.Version = 1
	}
	if err := ValidateThesisPositionLifecycle(v); err != nil {
		return err
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingSymbol, existingState string
	err = tx.QueryRow(`SELECT symbol,state FROM thesis_position_lifecycles WHERE thesis_id=?`, v.ThesisID).Scan(&existingSymbol, &existingState)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if existingSymbol != v.Symbol {
			return fmt.Errorf("thesis lifecycle symbol is immutable")
		}
		if !validThesisPositionTransition(ThesisPositionState(existingState), v.State) {
			return fmt.Errorf("invalid thesis lifecycle transition %s to %s", existingState, v.State)
		}
	}
	payload, _ := json.Marshal(v)
	_, err = tx.Exec(`INSERT INTO thesis_position_lifecycles(thesis_id,symbol,state,invalidation_price,primary_target_price,protection_price,position_quantity,avg_entry_price,opened_at,last_evaluated_at,version,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(thesis_id) DO UPDATE SET state=excluded.state,invalidation_price=excluded.invalidation_price,primary_target_price=excluded.primary_target_price,protection_price=excluded.protection_price,position_quantity=excluded.position_quantity,avg_entry_price=excluded.avg_entry_price,opened_at=excluded.opened_at,last_evaluated_at=excluded.last_evaluated_at,version=thesis_position_lifecycles.version+1,payload_json=excluded.payload_json`, v.ThesisID, v.Symbol, v.State, v.InvalidationPrice, v.PrimaryTargetPrice, v.ProtectionPrice, v.PositionQuantity, v.AvgEntryPrice, v.OpenedAt.Unix(), v.LastEvaluatedAt.Unix(), v.Version, string(payload))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) ThesisPositionLifecycleByID(id string) (ThesisPositionLifecycle, error) {
	var v ThesisPositionLifecycle
	var opened, evaluated int64
	err := d.QueryRow(`SELECT thesis_id,symbol,state,invalidation_price,primary_target_price,protection_price,position_quantity,avg_entry_price,opened_at,last_evaluated_at,version FROM thesis_position_lifecycles WHERE thesis_id=?`, strings.TrimSpace(id)).Scan(&v.ThesisID, &v.Symbol, &v.State, &v.InvalidationPrice, &v.PrimaryTargetPrice, &v.ProtectionPrice, &v.PositionQuantity, &v.AvgEntryPrice, &opened, &evaluated, &v.Version)
	if err != nil {
		return v, err
	}
	if opened > 0 {
		v.OpenedAt = time.Unix(opened, 0).UTC()
	}
	if evaluated > 0 {
		v.LastEvaluatedAt = time.Unix(evaluated, 0).UTC()
	}
	return v, ValidateThesisPositionLifecycle(v)
}
