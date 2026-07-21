package liveguard

import (
	"context"
	"fmt"

	"btc-agent/internal/exchange/live"
)

// CancelOrderAndConfirm treats the cancel acknowledgement as non-terminal until
// the exchange reports the final order state. A raced fill remains open as a
// partial fill so ledger reconciliation can apply it before terminal release.
func CancelOrderAndConfirm(ctx context.Context, order live.OrderStatus, canceler OrderCanceler, statusReader OrderStatusReader) (live.CancelOrderResult, live.OrderStatus, error) {
	if canceler == nil || statusReader == nil {
		return live.CancelOrderResult{}, order, fmt.Errorf("canceler and status reader required")
	}
	cancel, err := canceler.CancelOrder(ctx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
	if err != nil {
		return cancel, order, err
	}
	if !cancel.Canceled {
		return cancel, order, fmt.Errorf("cancel acknowledgement not confirmed")
	}
	remote, err := statusReader.OrderStatus(ctx, order.InstID, order.OrderID, order.ClientOrderID)
	if err != nil {
		return cancel, order, fmt.Errorf("post-cancel status unavailable: %w", err)
	}
	remote = withLocalOrderIdentity(remote, order)
	remote.Symbol = firstNonEmptyString(remote.Symbol, order.Symbol)
	remote.Source = order.Source
	remote.LayerIndex = order.LayerIndex
	filled := remote.AccumulatedFillSz
	if filled <= 0 {
		filled = remote.FilledQuantity
	}
	if filled > 0 {
		remote.Status = live.StatusPartialFill
		return cancel, remote, nil
	}
	if live.NormalizeOrderStatus(remote.Status) != live.StatusCancelled {
		return cancel, remote, fmt.Errorf("post-cancel status is not terminal: %s", remote.Status)
	}
	remote.Status = live.StatusCancelled
	return cancel, remote, nil
}
