package storage

import (
	"fmt"
	"math"
	"strings"
)

type ThesisPositionEvaluation struct {
	ThesisID      string              `json:"thesis_id"`
	Symbol        string              `json:"symbol"`
	CurrentPrice  float64             `json:"current_price"`
	CurrentState  ThesisPositionState `json:"current_state"`
	ProposedState ThesisPositionState `json:"proposed_state"`
	BlocksDCA     bool                `json:"blocks_dca"`
	SellAuthority bool                `json:"sell_authority"`
	Changed       bool                `json:"changed"`
	Reason        string              `json:"reason"`
}

// EvaluateThesisPositionLifecycle is pure and read-only. It never persists state
// and never grants SELL authority.
func EvaluateThesisPositionLifecycle(v ThesisPositionLifecycle, currentPrice float64) (ThesisPositionEvaluation, error) {
	if err := ValidateThesisPositionLifecycle(v); err != nil {
		return ThesisPositionEvaluation{}, err
	}
	if currentPrice <= 0 || math.IsNaN(currentPrice) || math.IsInf(currentPrice, 0) {
		return ThesisPositionEvaluation{}, fmt.Errorf("current price must be finite and positive")
	}
	out := ThesisPositionEvaluation{ThesisID: v.ThesisID, Symbol: v.Symbol, CurrentPrice: currentPrice, CurrentState: v.State, ProposedState: v.State, SellAuthority: false}
	out.BlocksDCA = !ThesisPositionStateAllowsDCA(v.State)
	if v.State == ThesisPositionClosed || v.State == ThesisPositionInvalidatedReview || v.State == ThesisPositionManualReview {
		return out, nil
	}
	if v.InvalidationPrice > 0 && currentPrice <= v.InvalidationPrice {
		out.ProposedState = ThesisPositionInvalidatedReview
		out.BlocksDCA = true
		out.Changed = v.State != out.ProposedState
		out.Reason = fmt.Sprintf("price %.8f reached invalidation %.8f; manual review required; no automatic SELL", currentPrice, v.InvalidationPrice)
		return out, nil
	}
	out.Reason = "invalidation not reached"
	return out, nil
}

func (d *DB) EvaluateThesisPositionLifecycleByID(thesisID string, currentPrice float64) (ThesisPositionEvaluation, error) {
	v, err := d.ThesisPositionLifecycleByID(strings.TrimSpace(thesisID))
	if err != nil {
		return ThesisPositionEvaluation{}, err
	}
	return EvaluateThesisPositionLifecycle(v, currentPrice)
}
