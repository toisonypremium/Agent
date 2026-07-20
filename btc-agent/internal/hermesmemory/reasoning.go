package hermesmemory

import (
	"encoding/json"
	"strings"
	"time"
)

type ReasoningRecord struct {
	EpisodeID        string    `json:"episode_id"`
	GeneratedAt      time.Time `json:"generated_at"`
	Facts            []string  `json:"facts"`
	Inferences       []string  `json:"inferences"`
	Contradictions   []string  `json:"contradictions,omitempty"`
	Unknowns         []string  `json:"unknowns,omitempty"`
	Alternatives     []string  `json:"alternatives,omitempty"`
	Conclusion       string    `json:"conclusion"`
	Confidence       float64   `json:"confidence"`
	ConfidenceLimits []string  `json:"confidence_limits,omitempty"`
	Authority        string    `json:"authority"`
}

type Prediction struct {
	PredictionID   string     `json:"prediction_id"`
	EpisodeID      string     `json:"episode_id"`
	CreatedAt      time.Time  `json:"created_at"`
	DueAt          time.Time  `json:"due_at"`
	Horizon        string     `json:"horizon"`
	Symbol         string     `json:"symbol"`
	ExpectedState  string     `json:"expected_state"`
	BasePrice      float64    `json:"base_price"`
	ExpectedReturn float64    `json:"expected_return"`
	Confidence     float64    `json:"confidence"`
	Status         string     `json:"status"`
	OutcomeReturn  *float64   `json:"outcome_return,omitempty"`
	OutcomeAt      *time.Time `json:"outcome_at,omitempty"`
	SquaredError   *float64   `json:"squared_error,omitempty"`
}

func BuildReasoning(e Episode, ctx Context, alternatives []string) ReasoningRecord {
	facts := append([]string{}, e.Facts...)
	inferences := append([]string{}, e.Inferences...)
	unknowns := append([]string{}, e.Unknowns...)
	contradictions := append([]string{}, ctx.Contradictions...)
	if len(contradictions) > 0 {
		unknowns = append(unknowns, "historical evidence conflicts with current state")
	}
	return ReasoningRecord{EpisodeID: e.EpisodeID, GeneratedAt: time.Now().UTC(), Facts: unique(facts), Inferences: unique(inferences), Contradictions: unique(contradictions), Unknowns: unique(unknowns), Alternatives: unique(alternatives), Conclusion: e.Conclusion, Confidence: ctx.CalibratedConfidence, ConfidenceLimits: unique(append(e.ConfidenceLimits, ctx.ConfidenceLimits...)), Authority: "deterministic_engine_only"}
}

func EncodeReasoning(r ReasoningRecord) string { b, _ := json.Marshal(r); return string(b) }

func NormalizePrediction(p Prediction) Prediction {
	p.Symbol = strings.ToUpper(strings.TrimSpace(p.Symbol))
	p.Horizon = strings.ToLower(strings.TrimSpace(p.Horizon))
	if p.Confidence < 0 {
		p.Confidence = 0
	}
	if p.Confidence > 1 {
		p.Confidence = 1
	}
	if p.Status == "" {
		p.Status = "PENDING"
	}
	return p
}

func ScorePrediction(p Prediction, actualReturn float64, at time.Time) Prediction {
	p = NormalizePrediction(p)
	p.OutcomeReturn = &actualReturn
	p.OutcomeAt = &at
	p.Status = "SCORED"
	err := actualReturn - p.ExpectedReturn
	p.SquaredError = new(float64)
	*p.SquaredError = err * err
	return p
}
