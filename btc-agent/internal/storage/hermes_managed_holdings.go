package storage

import (
	"btc-agent/internal/exchange/live"
	"encoding/json"
	"strings"
	"time"
)

type HermesManagedHolding struct {
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
	if h.AdoptedAt.IsZero() {
		h.AdoptedAt = time.Now().UTC()
	}
	h.UpdatedAt = time.Now().UTC()
	b, _ := json.Marshal(h)
	_, e := d.Exec(`INSERT INTO hermes_managed_holdings(symbol,inst_id,quantity,avg_entry_price,adopted_at,updated_at,source,payload_json) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(symbol) DO UPDATE SET inst_id=excluded.inst_id,quantity=excluded.quantity,avg_entry_price=excluded.avg_entry_price,updated_at=excluded.updated_at,source=excluded.source,payload_json=excluded.payload_json`, h.Symbol, h.InstID, h.Quantity, h.AvgEntryPrice, h.AdoptedAt.Unix(), h.UpdatedAt.Unix(), h.Source, string(b))
	return e
}
func (d *DB) HermesManagedHoldings() ([]HermesManagedHolding, error) {
	rows, e := d.Query(`SELECT symbol,inst_id,quantity,avg_entry_price,adopted_at,updated_at,source FROM hermes_managed_holdings WHERE quantity>0 ORDER BY symbol`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HermesManagedHolding{}
	for rows.Next() {
		var h HermesManagedHolding
		var a, u int64
		if e = rows.Scan(&h.Symbol, &h.InstID, &h.Quantity, &h.AvgEntryPrice, &a, &u, &h.Source); e != nil {
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
	return live.LivePosition{Symbol: h.Symbol, InstID: h.InstID, Quantity: h.Quantity, AvgEntryPrice: h.AvgEntryPrice, CostBasis: h.Quantity * h.AvgEntryPrice, OpenedAt: h.AdoptedAt.Unix(), UpdatedAt: h.UpdatedAt.Unix()}
}
