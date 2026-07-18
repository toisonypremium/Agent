package storage

import (
	"btc-agent/internal/exchange/live"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type HermesManagedHolding struct {
	ThesisID      string    `json:"thesis_id,omitempty"`
	Symbol        string    `json:"symbol"`
	InstID        string    `json:"inst_id"`
	Quantity      float64   `json:"quantity"`
	AvgEntryPrice float64   `json:"avg_entry_price"`
	AdoptedAt     time.Time `json:"adopted_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Source        string    `json:"source"`
}

func (d *DB) SaveHermesManagedHolding(h HermesManagedHolding) error {
	h.Symbol = strings.ToUpper(h.Symbol)
	h.ThesisID = strings.TrimSpace(h.ThesisID)
	if h.AdoptedAt.IsZero() {
		h.AdoptedAt = time.Now().UTC()
	}
	h.UpdatedAt = time.Now().UTC()
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingThesis string
	err = tx.QueryRow(`SELECT COALESCE(thesis_id,'') FROM hermes_managed_holdings WHERE symbol=?`, h.Symbol).Scan(&existingThesis)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if existingThesis != "" && h.ThesisID != "" && existingThesis != h.ThesisID {
		return fmt.Errorf("Hermes managed holding thesis conflict: existing=%s incoming=%s", existingThesis, h.ThesisID)
	}
	if h.ThesisID == "" {
		h.ThesisID = existingThesis
	}
	b, _ := json.Marshal(h)
	_, err = tx.Exec(`INSERT INTO hermes_managed_holdings(symbol,inst_id,quantity,avg_entry_price,adopted_at,updated_at,source,payload_json,thesis_id) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(symbol) DO UPDATE SET inst_id=excluded.inst_id,quantity=excluded.quantity,avg_entry_price=excluded.avg_entry_price,updated_at=excluded.updated_at,source=excluded.source,payload_json=excluded.payload_json,thesis_id=excluded.thesis_id`, h.Symbol, h.InstID, h.Quantity, h.AvgEntryPrice, h.AdoptedAt.Unix(), h.UpdatedAt.Unix(), h.Source, string(b), nullableString(h.ThesisID))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) HermesManagedHoldings() ([]HermesManagedHolding, error) {
	rows, e := d.Query(`SELECT symbol,inst_id,quantity,avg_entry_price,adopted_at,updated_at,source,COALESCE(thesis_id,'') FROM hermes_managed_holdings WHERE quantity>0 ORDER BY symbol`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HermesManagedHolding{}
	for rows.Next() {
		var h HermesManagedHolding
		var a, u int64
		if e = rows.Scan(&h.Symbol, &h.InstID, &h.Quantity, &h.AvgEntryPrice, &a, &u, &h.Source, &h.ThesisID); e != nil {
			return nil, e
		}
		h.AdoptedAt = time.Unix(a, 0)
		h.UpdatedAt = time.Unix(u, 0)
		out = append(out, h)
	}
	return out, rows.Err()
}
func (d *DB) DeleteHermesManagedHolding(symbol string) error {
	_, e := d.Exec(`DELETE FROM hermes_managed_holdings WHERE symbol=?`, strings.ToUpper(symbol))
	return e
}
func managedPosition(h HermesManagedHolding) live.LivePosition {
	return live.LivePosition{ThesisID: h.ThesisID, Symbol: h.Symbol, InstID: h.InstID, Quantity: h.Quantity, AvgEntryPrice: h.AvgEntryPrice, CostBasis: h.Quantity * h.AvgEntryPrice, OpenedAt: h.AdoptedAt.Unix(), UpdatedAt: h.UpdatedAt.Unix()}
}
