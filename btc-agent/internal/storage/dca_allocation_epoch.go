package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type DCAAllocationEntry struct {
	ThesisID   string  `json:"thesis_id"`
	Symbol     string  `json:"symbol"`
	Ratio      float64 `json:"ratio"`
	AmountUSDT float64 `json:"amount_usdt"`
}
type DCAAllocationEpoch struct {
	ID                    int64                `json:"id"`
	IdempotencyKey        string               `json:"idempotency_key"`
	ObservedAvailableUSDT float64              `json:"observed_available_usdt"`
	EnvelopeUSDT          float64              `json:"envelope_usdt"`
	NetNewUSDT            float64              `json:"net_new_usdt"`
	ObservedAt            time.Time            `json:"observed_at"`
	CreatedAt             time.Time            `json:"created_at"`
	Allocations           []DCAAllocationEntry `json:"allocations"`
}
type DCAAllocationEpochRequest struct {
	IdempotencyKey                                  string
	ObservedAvailableUSDT, EnvelopeUSDT, NetNewUSDT float64
	ObservedAt                                      time.Time
}

var dcaAllocationTargets = []DCAAllocationEntry{{"thesis-eth", "ETHUSDT", .40, 0}, {"thesis-link", "LINKUSDT", .35, 0}, {"thesis-virtual", "VIRTUALUSDT", .25, 0}}

func ValidateDCAAllocationEpochRequest(r DCAAllocationEpochRequest) error {
	if strings.TrimSpace(r.IdempotencyKey) == "" {
		return fmt.Errorf("allocation idempotency key required")
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("allocation observed time required")
	}
	for name, value := range map[string]float64{"observed_available_usdt": r.ObservedAvailableUSDT, "envelope_usdt": r.EnvelopeUSDT, "net_new_usdt": r.NetNewUSDT} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%s must be finite and non-negative", name)
		}
	}
	if r.EnvelopeUSDT > r.ObservedAvailableUSDT*.8+1e-9 {
		return fmt.Errorf("allocation envelope exceeds 80 percent verified USDT")
	}
	return nil
}
func (d *DB) CreateDCAAllocationEpoch(r DCAAllocationEpochRequest) (DCAAllocationEpoch, bool, error) {
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	r.ObservedAt = r.ObservedAt.UTC()
	if err := ValidateDCAAllocationEpochRequest(r); err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	alloc := make([]DCAAllocationEntry, len(dcaAllocationTargets))
	copy(alloc, dcaAllocationTargets)
	for i := range alloc {
		alloc[i].AmountUSDT = r.NetNewUSDT * alloc[i].Ratio
	}
	payload, err := json.Marshal(struct {
		DCAAllocationEpochRequest
		Allocations []DCAAllocationEntry `json:"allocations"`
	}{r, alloc})
	if err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	tx, err := d.Begin()
	if err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	defer tx.Rollback()
	var id int64
	var existing string
	err = tx.QueryRow(`SELECT id,payload_json FROM dca_allocation_epochs WHERE idempotency_key=?`, r.IdempotencyKey).Scan(&id, &existing)
	if err == nil {
		if existing != string(payload) {
			return DCAAllocationEpoch{}, false, fmt.Errorf("allocation idempotency payload conflict")
		}
		out, err := loadDCAAllocationEpoch(tx, id)
		return out, false, err
	}
	if err != sql.ErrNoRows {
		return DCAAllocationEpoch{}, false, err
	}
	res, err := tx.Exec(`INSERT INTO dca_allocation_epochs(idempotency_key,observed_available_usdt,envelope_usdt,net_new_usdt,observed_at,created_at,payload_json) VALUES(?,?,?,?,?,?,?)`, r.IdempotencyKey, r.ObservedAvailableUSDT, r.EnvelopeUSDT, r.NetNewUSDT, r.ObservedAt.Unix(), time.Now().UTC().Unix(), string(payload))
	if err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	for _, a := range alloc {
		if _, err := tx.Exec(`INSERT INTO dca_allocation_entries(epoch_id,thesis_id,symbol,allocation_ratio,amount_usdt) VALUES(?,?,?,?,?)`, id, a.ThesisID, a.Symbol, a.Ratio, a.AmountUSDT); err != nil {
			return DCAAllocationEpoch{}, false, err
		}
	}
	out, err := loadDCAAllocationEpoch(tx, id)
	if err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return DCAAllocationEpoch{}, false, err
	}
	return out, true, nil
}

type allocationQuery interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

func loadDCAAllocationEpoch(q allocationQuery, id int64) (DCAAllocationEpoch, error) {
	var e DCAAllocationEpoch
	var observed, created int64
	if err := q.QueryRow(`SELECT id,idempotency_key,observed_available_usdt,envelope_usdt,net_new_usdt,observed_at,created_at FROM dca_allocation_epochs WHERE id=?`, id).Scan(&e.ID, &e.IdempotencyKey, &e.ObservedAvailableUSDT, &e.EnvelopeUSDT, &e.NetNewUSDT, &observed, &created); err != nil {
		return e, err
	}
	e.ObservedAt = time.Unix(observed, 0).UTC()
	e.CreatedAt = time.Unix(created, 0).UTC()
	rows, err := q.Query(`SELECT thesis_id,symbol,allocation_ratio,amount_usdt FROM dca_allocation_entries WHERE epoch_id=? ORDER BY thesis_id`, id)
	if err != nil {
		return e, err
	}
	defer rows.Close()
	for rows.Next() {
		var a DCAAllocationEntry
		if err := rows.Scan(&a.ThesisID, &a.Symbol, &a.Ratio, &a.AmountUSDT); err != nil {
			return e, err
		}
		e.Allocations = append(e.Allocations, a)
	}
	return e, rows.Err()
}

// ApplyDCAAllocationEpochToTheses is the sole transaction that turns an
// immutable allocation record into thesis capital. It has no order authority.
func (d *DB) ApplyDCAAllocationEpochToTheses(epochID int64) (bool, error) {
	if epochID <= 0 {
		return false, fmt.Errorf("allocation epoch id required")
	}
	tx, err := d.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var epochExists int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM dca_allocation_epochs WHERE id=?`, epochID).Scan(&epochExists); err != nil {
		return false, err
	}
	if epochExists != 1 {
		return false, fmt.Errorf("allocation epoch not found: %d", epochID)
	}
	rows, err := tx.Query(`SELECT thesis_id,symbol,amount_usdt FROM dca_allocation_entries WHERE epoch_id=? ORDER BY thesis_id`, epochID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	type entry struct {
		thesisID, symbol string
		amount           float64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.thesisID, &e.symbol, &e.amount); err != nil {
			return false, err
		}
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if len(entries) != 3 {
		return false, fmt.Errorf("allocation epoch must have exactly three thesis entries")
	}
	var applied int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM dca_allocation_entry_applications WHERE epoch_id=?`, epochID).Scan(&applied); err != nil {
		return false, err
	}
	if applied == len(entries) {
		return false, nil
	}
	if applied != 0 {
		return false, fmt.Errorf("partial allocation epoch application: %d", epochID)
	}
	now := time.Now().UTC()
	for _, e := range entries {
		var symbol string
		var max, remaining float64
		err = tx.QueryRow(`SELECT symbol,max_exposure_usdt,remaining_dca_usdt FROM thesis_capital_ledgers WHERE thesis_id=?`, e.thesisID).Scan(&symbol, &max, &remaining)
		if err == sql.ErrNoRows {
			l := ThesisCapitalLedger{ThesisID: e.thesisID, Symbol: e.symbol, MaxExposureUSDT: e.amount, RemainingDCAUSDT: e.amount, Status: "ALLOCATED", Version: 1, CreatedAt: now, UpdatedAt: now}
			payload, e2 := json.Marshal(l)
			if e2 != nil {
				return false, e2
			}
			if _, e2 = tx.Exec(`INSERT INTO thesis_capital_ledgers(thesis_id,symbol,max_exposure_usdt,reserved_usdt,filled_usdt,remaining_dca_usdt,status,version,created_at,updated_at,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, l.ThesisID, l.Symbol, l.MaxExposureUSDT, 0, 0, l.RemainingDCAUSDT, l.Status, l.Version, l.CreatedAt.Unix(), l.UpdatedAt.Unix(), string(payload)); e2 != nil {
				return false, e2
			}
		} else if err != nil {
			return false, err
		} else if !strings.EqualFold(symbol, e.symbol) {
			return false, fmt.Errorf("allocation thesis symbol conflict: thesis_id=%s", e.thesisID)
		} else {
			if _, err = tx.Exec(`UPDATE thesis_capital_ledgers SET max_exposure_usdt=max_exposure_usdt+?,remaining_dca_usdt=remaining_dca_usdt+?,updated_at=?,version=version+1 WHERE thesis_id=?`, e.amount, e.amount, now.Unix(), e.thesisID); err != nil {
				return false, err
			}
		}
		if _, err = tx.Exec(`INSERT INTO dca_allocation_entry_applications(epoch_id,thesis_id,applied_at) VALUES(?,?,?)`, epochID, e.thesisID, now.Unix()); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (d *DB) LatestDCAAllocationEpoch() (DCAAllocationEpoch, error) {
	var id int64
	if err := d.QueryRow(`SELECT id FROM dca_allocation_epochs ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		return DCAAllocationEpoch{}, err
	}
	return loadDCAAllocationEpoch(d.DB, id)
}
