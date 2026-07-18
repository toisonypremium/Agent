package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/microstructure"
)

type EquityRiskState struct {
	UpdatedAt          time.Time `json:"updated_at"`
	CurrentEquity      float64   `json:"current_equity"`
	HighWaterMark      float64   `json:"high_water_mark"`
	UnrealizedDrawdown float64   `json:"unrealized_drawdown"`
	DrawdownPct        float64   `json:"drawdown_pct"`
}
type DailyEquityBasis struct {
	DayUTC time.Time `json:"day_utc"`
	Equity float64   `json:"equity"`
}

type ExitPeakState struct {
	Symbol      string    `json:"symbol"`
	Peak        float64   `json:"peak"`
	TrailActive bool      `json:"trail_active"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type ProtectionStatus struct {
	Name     string    `json:"name"`
	Symbol   string    `json:"symbol,omitempty"`
	Active   bool      `json:"active"`
	UnlockAt time.Time `json:"unlock_at,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}
type FillMarkout struct {
	EventID        int64     `json:"event_id"`
	HorizonMinutes int       `json:"horizon_minutes"`
	MarkPrice      float64   `json:"mark_price"`
	MarkoutPct     float64   `json:"markout_pct"`
	MeasuredAt     time.Time `json:"measured_at"`
}
type ReplayState struct {
	Events    int                       `json:"events"`
	Positions map[string]ReplayPosition `json:"positions"`
	Checksum  string                    `json:"checksum"`
}
type ReplayPosition struct {
	Quantity float64 `json:"quantity"`
	Cost     float64 `json:"cost"`
}

func (d *DB) UpdateEquityRiskState(equity float64, now time.Time) (EquityRiskState, error) {
	if equity <= 0 {
		return EquityRiskState{}, fmt.Errorf("equity must be positive")
	}
	s, err := d.EquityRiskState()
	if err != nil && err != sql.ErrNoRows {
		return s, err
	}
	if s.HighWaterMark < equity {
		s.HighWaterMark = equity
	}
	s.CurrentEquity = equity
	s.UnrealizedDrawdown = s.HighWaterMark - equity
	if s.UnrealizedDrawdown < 0 {
		s.UnrealizedDrawdown = 0
	}
	if s.HighWaterMark > 0 {
		s.DrawdownPct = s.UnrealizedDrawdown / s.HighWaterMark
	}
	s.UpdatedAt = now
	_, err = d.Exec(`INSERT OR REPLACE INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('equity',?,?)`, now.Unix(), mustJSONRisk(s))
	return s, err
}

// DailyOpeningEquity returns the first valid equity recorded for a UTC day.
// INSERT OR IGNORE makes concurrent callers converge on one immutable basis.
func (d *DB) DailyOpeningEquity(now time.Time, currentEquity float64) (DailyEquityBasis, error) {
	if currentEquity <= 0 {
		return DailyEquityBasis{}, fmt.Errorf("daily opening equity must be positive")
	}
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	key := "daily_equity_basis:" + day.Format("2006-01-02")
	basis := DailyEquityBasis{DayUTC: day, Equity: currentEquity}
	b, err := json.Marshal(basis)
	if err != nil {
		return DailyEquityBasis{}, err
	}
	if _, err = d.Exec(`INSERT OR IGNORE INTO hermes_runtime_state(key,updated_at,payload_json) VALUES(?,?,?)`, key, now.Unix(), string(b)); err != nil {
		return DailyEquityBasis{}, err
	}
	var js string
	if err = d.QueryRow(`SELECT payload_json FROM hermes_runtime_state WHERE key=?`, key).Scan(&js); err != nil {
		return DailyEquityBasis{}, err
	}
	if err = json.Unmarshal([]byte(js), &basis); err != nil {
		return DailyEquityBasis{}, err
	}
	if basis.Equity <= 0 || !basis.DayUTC.Equal(day) {
		return DailyEquityBasis{}, fmt.Errorf("invalid persisted daily equity basis")
	}
	return basis, nil
}

func (d *DB) EquityRiskState() (EquityRiskState, error) {
	var ts int64
	var js string
	err := d.QueryRow(`SELECT updated_at,payload_json FROM hermes_runtime_state WHERE key='equity'`).Scan(&ts, &js)
	var s EquityRiskState
	if err == nil {
		err = json.Unmarshal([]byte(js), &s)
		s.UpdatedAt = time.Unix(ts, 0)
	}
	return s, err
}
func (d *DB) SaveExitPeakStates(states []ExitPeakState) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM hermes_exit_peaks`); err != nil {
		return err
	}
	for _, s := range states {
		if _, err = tx.Exec(`INSERT INTO hermes_exit_peaks(symbol,peak,trail_active,updated_at) VALUES(?,?,?,?)`, strings.ToUpper(s.Symbol), s.Peak, s.TrailActive, s.UpdatedAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) ExitPeakStates() ([]ExitPeakState, error) {
	rows, err := d.Query(`SELECT symbol,peak,trail_active,updated_at FROM hermes_exit_peaks ORDER BY symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExitPeakState{}
	for rows.Next() {
		var s ExitPeakState
		var ts int64
		if err = rows.Scan(&s.Symbol, &s.Peak, &s.TrailActive, &ts); err != nil {
			return nil, err
		}
		s.UpdatedAt = time.Unix(ts, 0)
		out = append(out, s)
	}
	return out, rows.Err()
}
func (d *DB) SaveMMCalibration(state microstructure.CalibrationState, now time.Time) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT OR REPLACE INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('mm_footprint_calibration',?,?)`, now.Unix(), string(b))
	return err
}

func (d *DB) MMCalibration() (microstructure.CalibrationState, error) {
	var js string
	err := d.QueryRow(`SELECT payload_json FROM hermes_runtime_state WHERE key='mm_footprint_calibration'`).Scan(&js)
	state := microstructure.NewCalibrationState()
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal([]byte(js), &state); err != nil {
		return microstructure.NewCalibrationState(), err
	}
	return state, nil
}

func (d *DB) SaveProtectionStatuses(items []ProtectionStatus, now time.Time) error {
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT OR REPLACE INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('protections',?,?)`, now.Unix(), string(b))
	return err
}
func (d *DB) ProtectionStatuses() ([]ProtectionStatus, error) {
	var js string
	err := d.QueryRow(`SELECT payload_json FROM hermes_runtime_state WHERE key='protections'`).Scan(&js)
	var out []ProtectionStatus
	if err == nil {
		err = json.Unmarshal([]byte(js), &out)
	}
	return out, err
}
func (d *DB) SaveFillMarkout(m FillMarkout) error {
	_, err := d.Exec(`INSERT OR REPLACE INTO execution_markouts(event_id,horizon_minutes,mark_price,markout_pct,measured_at) VALUES(?,?,?,?,?)`, m.EventID, m.HorizonMinutes, m.MarkPrice, m.MarkoutPct, m.MeasuredAt.Unix())
	return err
}
func (d *DB) ReplayHermesState() (ReplayState, error) {
	rows, err := d.Query(`SELECT e.id,UPPER(e.symbol),UPPER(e.side),e.delta_quantity,e.notional_delta,e.fill_price FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' ORDER BY e.timestamp,e.id`)
	if err != nil {
		return ReplayState{}, err
	}
	defer rows.Close()
	r := ReplayState{Positions: map[string]ReplayPosition{}}
	for rows.Next() {
		var id int64
		var sym, side string
		var q, n, p float64
		if err = rows.Scan(&id, &sym, &side, &q, &n, &p); err != nil {
			return r, err
		}
		if n <= 0 {
			n = q * p
		}
		v := r.Positions[sym]
		if side == "BUY" {
			v.Quantity += q
			v.Cost += n
		} else if side == "SELL" && v.Quantity > 0 {
			sq := q
			if sq > v.Quantity {
				sq = v.Quantity
			}
			avg := v.Cost / v.Quantity
			v.Quantity -= sq
			v.Cost -= avg * sq
		}
		if v.Quantity < 1e-12 {
			v = ReplayPosition{}
		}
		r.Positions[sym] = v
		r.Events++
	}
	if err = rows.Err(); err != nil {
		return r, err
	}
	keys := make([]string, 0, len(r.Positions))
	for k := range r.Positions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		v := r.Positions[k]
		fmt.Fprintf(h, "%s|%.12f|%.12f\n", k, v.Quantity, v.Cost)
	}
	r.Checksum = hex.EncodeToString(h.Sum(nil))
	return r, nil
}
func mustJSONRisk(v any) string { b, _ := json.Marshal(v); return string(b) }
