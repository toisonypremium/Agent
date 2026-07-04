package liveguard

import (
	"context"
	"fmt"
	"time"

	"btc-agent/internal/exchange/live"
)

type OrderStatusReader interface {
	OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error)
	PendingOrders(ctx context.Context, instID string) ([]live.OrderStatus, error)
}

type ReconcileResult struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Checked     int                `json:"checked"`
	Updated     int                `json:"updated"`
	Unknown     int                `json:"unknown"`
	Orders      []live.OrderStatus `json:"orders"`
	Summary     string             `json:"summary"`
}

func ReconcileOrders(ctx context.Context, reader OrderStatusReader, open []live.OrderStatus) ReconcileResult {
	res := ReconcileResult{
		GeneratedAt: time.Now(),
		Orders:      []live.OrderStatus{},
	}

	if len(open) == 0 {
		res.Summary = "no local live orders to reconcile"
		return res
	}

	res.Checked = len(open)
	if reader == nil {
		for _, o := range open {
			o.Status = live.StatusUnknownNeedsManualCheck
			res.Unknown++
			res.Orders = append(res.Orders, o)
		}
		res.Summary = fmt.Sprintf("reconciled %d orders: updated %d, unknown %d", res.Checked, res.Updated, res.Unknown)
		return res
	}
	for _, o := range open {
		remote, err := reader.OrderStatus(ctx, o.InstID, o.OrderID, o.ClientOrderID)
		if err != nil {
			o.Status = live.StatusUnknownNeedsManualCheck
			res.Unknown++
			res.Orders = append(res.Orders, o)
			continue
		}

		if remote.Status != o.Status {
			res.Updated++
		}
		res.Orders = append(res.Orders, remote)
	}

	res.Summary = fmt.Sprintf("reconciled %d orders: updated %d, unknown %d", res.Checked, res.Updated, res.Unknown)
	return res
}
