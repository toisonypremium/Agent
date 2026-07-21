package executionguard

import (
	"context"
	"fmt"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/runtime/ownership"
)

type Exchange interface {
	PlaceSpotLimitOrder(context.Context, live.LimitOrderRequest) (live.OrderResult, error)
	CancelOrder(context.Context, live.CancelOrderRequest) (live.CancelOrderResult, error)
}

type GuardedExchange struct {
	Exchange Exchange
	Manager  *ownership.Manager
	Lease    ownership.Lease
}

func (g GuardedExchange) verify(ctx context.Context) error {
	if g.Exchange == nil || g.Manager == nil {
		return fmt.Errorf("execution guard unavailable: %w", ownership.ErrNotOwner)
	}
	return g.Manager.Verify(ctx, g.Lease)
}

func (g GuardedExchange) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	if err := g.verify(ctx); err != nil {
		return live.OrderResult{}, err
	}
	return g.Exchange.PlaceSpotLimitOrder(ctx, req)
}

func (g GuardedExchange) CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	if err := g.verify(ctx); err != nil {
		return live.CancelOrderResult{}, err
	}
	return g.Exchange.CancelOrder(ctx, req)
}

func (g GuardedExchange) OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error) {
	reader, ok := g.Exchange.(interface {
		OrderStatus(context.Context, string, string, string) (live.OrderStatus, error)
	})
	if !ok {
		return live.OrderStatus{}, fmt.Errorf("order status reader unavailable")
	}
	return reader.OrderStatus(ctx, instID, orderID, clientOrderID)
}

func (g GuardedExchange) PendingOrders(ctx context.Context, instID string) ([]live.OrderStatus, error) {
	reader, ok := g.Exchange.(interface {
		PendingOrders(context.Context, string) ([]live.OrderStatus, error)
	})
	if !ok {
		return nil, fmt.Errorf("pending order reader unavailable")
	}
	return reader.PendingOrders(ctx, instID)
}
