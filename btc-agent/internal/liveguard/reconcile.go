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

const (
	ReconcileClean = "RECONCILE_CLEAN"
	ReconcileWarn  = "RECONCILE_WARN"
	ReconcileBlock = "RECONCILE_BLOCK"
)

type ReconcileResult struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Checked     int                   `json:"checked"`
	Updated     int                   `json:"updated"`
	Unknown     int                   `json:"unknown"`
	Orders      []live.OrderStatus    `json:"orders"`
	Safety      ReconcileSafetyResult `json:"safety,omitempty"`
	Summary     string                `json:"summary"`
}

type ReconcileSafetyResult struct {
	Status    string   `json:"status"`
	LocalOpen int      `json:"local_open"`
	Unknown   int      `json:"unknown"`
	Blockers  []string `json:"blockers,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	Summary   string   `json:"summary"`
}

func ReconcileOrders(ctx context.Context, reader OrderStatusReader, open []live.OrderStatus) ReconcileResult {
	res := ReconcileResult{
		GeneratedAt: time.Now(),
		Orders:      []live.OrderStatus{},
	}

	if len(open) == 0 {
		res.Summary = "no local live orders to reconcile"
		res.Safety = ReconcileSafety(res)
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
		res.Safety = ReconcileSafety(res)
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
		remote = withLocalOrderIdentity(remote, o)

		if remote.Status != o.Status {
			res.Updated++
		}
		res.Orders = append(res.Orders, remote)
	}

	res.Summary = fmt.Sprintf("reconciled %d orders: updated %d, unknown %d", res.Checked, res.Updated, res.Unknown)
	res.Safety = ReconcileSafety(res)
	return res
}

func withLocalOrderIdentity(remote, local live.OrderStatus) live.OrderStatus {
	if remote.InstID == "" {
		remote.InstID = local.InstID
	}
	if remote.OrderID == "" {
		remote.OrderID = local.OrderID
	}
	if remote.ClientOrderID == "" {
		remote.ClientOrderID = local.ClientOrderID
	}
	return remote
}

func ReconcileSafety(result ReconcileResult) ReconcileSafetyResult {
	safety := ReconcileSafetyResult{Status: ReconcileClean, LocalOpen: result.Checked, Unknown: result.Unknown}
	if result.Unknown > 0 {
		safety.Blockers = append(safety.Blockers, fmt.Sprintf("%d live order status unknown", result.Unknown))
	}
	for _, order := range result.Orders {
		if order.Status == live.StatusUnknownNeedsManualCheck {
			safety.Blockers = append(safety.Blockers, fmt.Sprintf("%s/%s needs manual check", order.ClientOrderID, order.OrderID))
		}
		if live.NormalizeOrderStatus(order.Status) == live.StatusPartialFill || live.NormalizeOrderStatus(order.Status) == live.StatusFilled {
			filled := order.AccumulatedFillSz
			if filled == 0 {
				filled = order.FilledQuantity
			}
			if filled <= 0 || (order.AvgPrice <= 0 && order.Price <= 0) {
				safety.Blockers = append(safety.Blockers, fmt.Sprintf("%s/%s fill status missing fill quantity/price", order.ClientOrderID, order.OrderID))
			}
		}
	}
	safety.Blockers = uniqueHealthStrings(safety.Blockers)
	safety.Warnings = uniqueHealthStrings(safety.Warnings)
	if len(safety.Blockers) > 0 {
		safety.Status = ReconcileBlock
	} else if len(safety.Warnings) > 0 {
		safety.Status = ReconcileWarn
	}
	safety.Summary = fmt.Sprintf("%s: local_open=%d unknown=%d blockers=%d warnings=%d", safety.Status, safety.LocalOpen, safety.Unknown, len(safety.Blockers), len(safety.Warnings))
	return safety
}
