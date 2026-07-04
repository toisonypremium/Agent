package agent2

import "time"

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
	for _, a := range p.Assets {
		if a.State != StateActiveLimit {
			continue
		}
		for _, l := range a.Layers {
			out = append(out, PaperOrder{ID: now.Format("20060102150405") + "-" + a.Symbol + "-L" + string(rune('0'+l.Index)), Timestamp: now, Symbol: a.Symbol, Side: "BUY", Layer: l.Index, Price: l.Price, Quantity: l.Quantity, Notional: l.Notional, InvalidationPrice: a.Invalidation, Status: "OPEN", ExpiresAt: now.Add(time.Duration(expiryHours) * time.Hour), Reason: a.Reason})
		}
	}
	return out
}
