package operatorchange

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Action string

const (
	Resume              Action = "RESUME"
	SetHermesObserve    Action = "SET_HERMES_OBSERVE"
	SetHermesShadow     Action = "SET_HERMES_SHADOW"
	SetHermesCanary     Action = "SET_HERMES_CANARY"
	ReduceRiskCaps      Action = "REDUCE_RISK_CAPS"
	IncreaseRiskCaps    Action = "INCREASE_RISK_CAPS"
	DisableCircuitTimer Action = "DISABLE_CIRCUIT_TIMER"
	EnableCircuitTimer  Action = "ENABLE_CIRCUIT_TIMER"
)

type Status string

const (
	Pending   Status = "PENDING"
	Confirmed Status = "CONFIRMED"
	Applied   Status = "APPLIED"
	Cancelled Status = "CANCELLED"
	Expired   Status = "EXPIRED"
)

type Request struct {
	ID                    string             `json:"id"`
	Action                Action             `json:"action"`
	Requester             string             `json:"requester"`
	Reason                string             `json:"reason"`
	Before                map[string]float64 `json:"before,omitempty"`
	After                 map[string]float64 `json:"after,omitempty"`
	SafetySnapshotID      string             `json:"safety_snapshot_id"`
	SafetySnapshotSHA256  string             `json:"safety_snapshot_sha256"`
	CreatedAt             time.Time          `json:"created_at"`
	ExpiresAt             time.Time          `json:"expires_at"`
	RequiredConfirmations int                `json:"required_confirmations"`
	Confirmers            []string           `json:"confirmers,omitempty"`
	Status                Status             `json:"status"`
}

func Validate(r Request, now time.Time) error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Requester) == "" {
		return errors.New("identity and id required")
	}
	if len(strings.TrimSpace(r.Reason)) < 8 || len(r.Reason) > 500 {
		return errors.New("reason length invalid")
	}
	if r.CreatedAt.IsZero() || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.CreatedAt) || r.ExpiresAt.Sub(r.CreatedAt) > 30*time.Minute || !now.Before(r.ExpiresAt) {
		return errors.New("expiry invalid")
	}
	if len(r.SafetySnapshotID) < 8 || len(r.SafetySnapshotSHA256) != 64 {
		return errors.New("safety binding invalid")
	}
	switch r.Action {
	case Resume, SetHermesObserve, SetHermesShadow, SetHermesCanary, DisableCircuitTimer, EnableCircuitTimer:
		if r.RequiredConfirmations != 1 {
			return errors.New("one confirmation required")
		}
	case ReduceRiskCaps:
		if r.RequiredConfirmations != 1 {
			return errors.New("one confirmation required")
		}
		if err := validateCaps(r, false); err != nil {
			return err
		}
	case IncreaseRiskCaps:
		if r.RequiredConfirmations != 2 {
			return errors.New("dual control required")
		}
		if err := validateCaps(r, true); err != nil {
			return err
		}
	default:
		return errors.New("action forbidden")
	}
	return nil
}
func validateCaps(r Request, increase bool) error {
	if len(r.After) == 0 {
		return errors.New("caps required")
	}
	for k, v := range r.After {
		before, ok := r.Before[k]
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return fmt.Errorf("invalid cap %s", k)
		}
		if increase && v <= before {
			return fmt.Errorf("cap %s does not increase", k)
		}
		if !increase && v > before {
			return fmt.Errorf("cap %s increases", k)
		}
	}
	return nil
}
func Confirm(r Request, identity string) (Request, error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	if identity == "" {
		return r, errors.New("identity required")
	}
	for _, x := range r.Confirmers {
		if strings.EqualFold(x, identity) {
			return r, errors.New("duplicate confirmer")
		}
	}
	if r.Action == IncreaseRiskCaps && strings.EqualFold(r.Requester, identity) {
		return r, errors.New("requester cannot approve increase")
	}
	r.Confirmers = append(r.Confirmers, identity)
	if len(r.Confirmers) >= r.RequiredConfirmations {
		r.Status = Confirmed
	}
	return r, nil
}
