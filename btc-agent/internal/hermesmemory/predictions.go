package hermesmemory

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const predictionSchema = `CREATE TABLE IF NOT EXISTS hermes_predictions(
 prediction_id TEXT PRIMARY KEY,
 episode_id TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 due_at INTEGER NOT NULL,
 horizon TEXT NOT NULL,
 symbol TEXT NOT NULL,
 expected_state TEXT NOT NULL,
 expected_return REAL NOT NULL,
 confidence REAL NOT NULL,
 status TEXT NOT NULL,
 payload_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hermes_predictions_pending ON hermes_predictions(status,due_at);
CREATE INDEX IF NOT EXISTS idx_hermes_predictions_symbol_created ON hermes_predictions(symbol,created_at DESC);`

type Calibration struct {
	Samples            int                          `json:"samples"`
	MeanSquaredError   float64                      `json:"mean_squared_error"`
	MeanAbsoluteError  float64                      `json:"mean_absolute_error"`
	Bias               float64                      `json:"bias"`
	AvgConfidence      float64                      `json:"avg_confidence"`
	OverconfidenceRate float64                      `json:"overconfidence_rate"`
	Quality            string                       `json:"quality"`
	ByHorizon          map[string]CalibrationBucket `json:"by_horizon,omitempty"`
}
type CalibrationBucket struct {
	Samples           int     `json:"samples"`
	MeanSquaredError  float64 `json:"mean_squared_error"`
	MeanAbsoluteError float64 `json:"mean_absolute_error"`
	Bias              float64 `json:"bias"`
}

func EnsurePredictions(db DB) error {
	if _, err := db.Exec(predictionSchema); err != nil {
		return err
	}
	// Repeated scheduler cycles are correlated observations, not independent
	// prediction samples. Keep the oldest pending cohort per symbol/horizon.
	if _, err := db.Exec(`DELETE FROM hermes_predictions WHERE status='PENDING' AND rowid NOT IN (SELECT MIN(rowid) FROM hermes_predictions WHERE status='PENDING' GROUP BY symbol,horizon)`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_hermes_predictions_one_pending ON hermes_predictions(symbol,horizon) WHERE status='PENDING'`)
	return err
}
func SavePrediction(db DB, p Prediction, dueAt time.Time) error {
	if err := EnsurePredictions(db); err != nil {
		return err
	}
	p = NormalizePrediction(p)
	if p.PredictionID == "" || p.EpisodeID == "" || p.Symbol == "" || p.Horizon == "" {
		return errors.New("prediction identity incomplete")
	}
	p.DueAt = dueAt
	if dueAt.IsZero() || !dueAt.After(p.CreatedAt) {
		return errors.New("prediction due_at must be after created_at")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO hermes_predictions(prediction_id,episode_id,created_at,due_at,horizon,symbol,expected_state,expected_return,confidence,status,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, p.PredictionID, p.EpisodeID, p.CreatedAt.Unix(), dueAt.Unix(), p.Horizon, p.Symbol, p.ExpectedState, p.ExpectedReturn, p.Confidence, p.Status, string(b))
	return err
}
func PendingPredictions(db DB, now time.Time, limit int) ([]Prediction, error) {
	if err := EnsurePredictions(db); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`SELECT payload_json FROM hermes_predictions WHERE status='PENDING' AND due_at<=? ORDER BY due_at LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Prediction{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p Prediction
		if json.Unmarshal([]byte(raw), &p) == nil {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}
func PersistScore(db DB, p Prediction) error {
	if p.Status != "SCORED" || p.OutcomeReturn == nil || p.SquaredError == nil {
		return errors.New("prediction not scored")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	r, err := db.Exec(`UPDATE hermes_predictions SET status='SCORED',payload_json=? WHERE prediction_id=? AND status='PENDING'`, string(b), p.PredictionID)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func LoadCalibration(db DB, limit int) (Calibration, error) {
	out := Calibration{Quality: "INSUFFICIENT_SAMPLE", ByHorizon: map[string]CalibrationBucket{}}
	if err := EnsurePredictions(db); err != nil {
		return out, err
	}
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.Query(`SELECT payload_json FROM hermes_predictions WHERE status='SCORED' ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	type acc struct {
		n                   int
		sq, abs, bias, conf float64
		over                int
	}
	total := acc{}
	by := map[string]acc{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		var p Prediction
		if json.Unmarshal([]byte(raw), &p) != nil || p.OutcomeReturn == nil {
			continue
		}
		e := *p.OutcomeReturn - p.ExpectedReturn
		a := by[p.Horizon]
		a.n++
		a.sq += e * e
		a.abs += math.Abs(e)
		a.bias += e
		by[p.Horizon] = a
		total.n++
		total.sq += e * e
		total.abs += math.Abs(e)
		total.bias += e
		total.conf += p.Confidence
		if p.Confidence >= .7 && math.Abs(e) > .05 {
			total.over++
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if total.n == 0 {
		return out, nil
	}
	out.Samples = total.n
	out.MeanSquaredError = total.sq / float64(total.n)
	out.MeanAbsoluteError = total.abs / float64(total.n)
	out.Bias = total.bias / float64(total.n)
	out.AvgConfidence = total.conf / float64(total.n)
	out.OverconfidenceRate = float64(total.over) / float64(total.n)
	if total.n >= 100 {
		out.Quality = "ADEQUATE"
	} else if total.n >= 20 {
		out.Quality = "LIMITED"
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := by[k]
		out.ByHorizon[k] = CalibrationBucket{a.n, a.sq / float64(a.n), a.abs / float64(a.n), a.bias / float64(a.n)}
	}
	return out, nil
}
func NewPrediction(episodeID, symbol, horizon, state string, basePrice, expectedReturn, confidence float64, created time.Time) (Prediction, time.Time, error) {
	if created.IsZero() {
		created = time.Now().UTC()
	}
	dur, err := parseHorizon(horizon)
	if err != nil {
		return Prediction{}, time.Time{}, err
	}
	p := NormalizePrediction(Prediction{PredictionID: fmt.Sprintf("%s:%s:%s", episodeID, strings.ToUpper(symbol), strings.ToLower(horizon)), EpisodeID: episodeID, CreatedAt: created, DueAt: created.Add(dur), Horizon: horizon, Symbol: symbol, ExpectedState: state, BasePrice: basePrice, ExpectedReturn: expectedReturn, Confidence: confidence})
	return p, created.Add(dur), nil
}
func parseHorizon(s string) (time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1h":
		return time.Hour, nil
	case "4h":
		return 4 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "3d":
		return 72 * time.Hour, nil
	case "7d":
		return 168 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported horizon %q", s)
	}
}
