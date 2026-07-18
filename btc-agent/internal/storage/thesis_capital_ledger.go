package storage

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type ThesisCapitalLedger struct {
	ThesisID         string    `json:"thesis_id"`
	Symbol           string    `json:"symbol"`
	MaxExposureUSDT  float64   `json:"max_exposure_usdt"`
	ReservedUSDT     float64   `json:"reserved_usdt"`
	FilledUSDT       float64   `json:"filled_usdt"`
	RemainingDCAUSDT float64   `json:"remaining_dca_usdt"`
	Status           string    `json:"status"`
	Version          int64     `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func ValidateThesisCapitalLedger(l ThesisCapitalLedger) error {
	if strings.TrimSpace(l.ThesisID) == "" {
		return fmt.Errorf("thesis id required")
	}
	if strings.TrimSpace(l.Symbol) == "" {
		return fmt.Errorf("thesis symbol required")
	}
	if strings.TrimSpace(l.Status) == "" {
		return fmt.Errorf("thesis status required")
	}
	for name, value := range map[string]float64{"max_exposure_usdt": l.MaxExposureUSDT, "reserved_usdt": l.ReservedUSDT, "filled_usdt": l.FilledUSDT, "remaining_dca_usdt": l.RemainingDCAUSDT} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%s must be finite and non-negative", name)
		}
	}
	if l.FilledUSDT+l.ReservedUSDT+l.RemainingDCAUSDT > l.MaxExposureUSDT+1e-9 {
		return fmt.Errorf("thesis capital exceeds max exposure")
	}
	return nil
}

func (d *DB) SaveThesisCapitalLedger(l ThesisCapitalLedger) error {
	l.ThesisID = strings.TrimSpace(l.ThesisID)
	l.Symbol = strings.ToUpper(strings.TrimSpace(l.Symbol))
	l.Status = strings.ToUpper(strings.TrimSpace(l.Status))
	if err := ValidateThesisCapitalLedger(l); err != nil {
		return err
	}
	now := time.Now().UTC()
	if l.CreatedAt.IsZero() {
		l.CreatedAt = now
	}
	if l.UpdatedAt.IsZero() {
		l.UpdatedAt = now
	}
	if l.Version <= 0 {
		l.Version = 1
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT INTO thesis_capital_ledgers(thesis_id,symbol,max_exposure_usdt,reserved_usdt,filled_usdt,remaining_dca_usdt,status,version,created_at,updated_at,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(thesis_id) DO UPDATE SET max_exposure_usdt=excluded.max_exposure_usdt,reserved_usdt=excluded.reserved_usdt,filled_usdt=excluded.filled_usdt,remaining_dca_usdt=excluded.remaining_dca_usdt,status=excluded.status,version=excluded.version,updated_at=excluded.updated_at,payload_json=excluded.payload_json WHERE thesis_capital_ledgers.symbol=excluded.symbol`, l.ThesisID, l.Symbol, l.MaxExposureUSDT, l.ReservedUSDT, l.FilledUSDT, l.RemainingDCAUSDT, l.Status, l.Version, l.CreatedAt.Unix(), l.UpdatedAt.Unix(), string(b))
	return err
}

func (d *DB) ThesisCapitalLedgerByID(thesisID string) (ThesisCapitalLedger, error) {
	var l ThesisCapitalLedger
	var created, updated int64
	var payload string
	err := d.QueryRow(`SELECT thesis_id,symbol,max_exposure_usdt,reserved_usdt,filled_usdt,remaining_dca_usdt,status,version,created_at,updated_at,payload_json FROM thesis_capital_ledgers WHERE thesis_id=?`, strings.TrimSpace(thesisID)).Scan(&l.ThesisID, &l.Symbol, &l.MaxExposureUSDT, &l.ReservedUSDT, &l.FilledUSDT, &l.RemainingDCAUSDT, &l.Status, &l.Version, &created, &updated, &payload)
	if err != nil {
		return l, err
	}
	l.CreatedAt, l.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	if err := ValidateThesisCapitalLedger(l); err != nil {
		return ThesisCapitalLedger{}, err
	}
	return l, nil
}

func (d *DB) ThesisCapitalLedgers() ([]ThesisCapitalLedger, error) {
	rows, err := d.Query(`SELECT thesis_id,symbol,max_exposure_usdt,reserved_usdt,filled_usdt,remaining_dca_usdt,status,version,created_at,updated_at,payload_json FROM thesis_capital_ledgers ORDER BY thesis_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ThesisCapitalLedger{}
	for rows.Next() {
		var l ThesisCapitalLedger
		var created, updated int64
		var payload string
		if err := rows.Scan(&l.ThesisID, &l.Symbol, &l.MaxExposureUSDT, &l.ReservedUSDT, &l.FilledUSDT, &l.RemainingDCAUSDT, &l.Status, &l.Version, &created, &updated, &payload); err != nil {
			return nil, err
		}
		l.CreatedAt, l.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
		if err := ValidateThesisCapitalLedger(l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
