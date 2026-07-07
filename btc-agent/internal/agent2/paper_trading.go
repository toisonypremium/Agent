package agent2

import (
	"fmt"
	"time"
)

type PaperOrder struct {
	ID                                           string    `json:"id"`
	Timestamp                                    time.Time `json:"timestamp"`
	Symbol                                       string    `json:"symbol"`
	Side                                         string    `json:"side"`
	Layer                                        int       `json:"layer"`
	Price, Quantity, Notional, InvalidationPrice float64
	Status                                       string    `json:"status"`
	ExpiresAt                                    time.Time `json:"expires_at"`
	Reason                                       string    `json:"reason"`
}

func OrdersFromPlan(p Plan, expiryHours int) []PaperOrder {
	out := []PaperOrder{}
	now := time.Now()
	stamp := now.Format("20060102150405.000000000")
	for assetIndex, a := range p.Assets {
		if a.State != StateActiveLimit {
			continue
		}
		for layerIndex, l := range a.Layers {
			id := fmt.Sprintf("%s-A%d-%d-%s-L%d", stamp, assetIndex, layerIndex, a.Symbol, l.Index)
			out = append(out, PaperOrder{ID: id, Timestamp: now, Symbol: a.Symbol, Side: "BUY", Layer: l.Index, Price: l.Price, Quantity: l.Quantity, Notional: l.Notional, InvalidationPrice: a.Invalidation, Status: "OPEN", ExpiresAt: now.Add(time.Duration(expiryHours) * time.Hour), Reason: a.Reason})
		}
	}
	return out
}
