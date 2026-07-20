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
