package simulator

import (
	"context"

	"btc-agent/internal/exchange/live"
)

// FillSimulation applies deterministic OHLCV fill assumptions to FakeOKX orders.
type FillSimulation struct {
	OKX          *FakeOKX
	MakerFeeRate float64
}

func (s *FillSimulation) ProcessCandle(ctx context.Context, instID string, high, low, close, volume float64) error {
	if s == nil || s.OKX == nil {
		return nil
	}
	feeRate := s.MakerFeeRate
	if feeRate <= 0 {
		feeRate = defaultMakerFeeRate
	}
	orders := s.openOrders(instID)
	for _, order := range orders {
		assumption := FillAssumption{FeeRate: feeRate}
		if order.Side == "buy" && low <= order.Price {
			assumption.BestAsk = order.Price
			assumption.FillPrice = order.Price
			assumption.FillQuantity = conservativeFillQuantity(order, low, order.Price)
		} else if order.Side == "sell" && high >= order.Price {
			assumption.BestBid = order.Price
			assumption.FillPrice = order.Price
			assumption.FillQuantity = conservativeFillQuantity(order, order.Price, high)
		} else {
			continue
		}
		if err := s.OKX.SimulateFill(order.ClientOrderID, assumption); err != nil {
			return err
		}
	}
	return nil
}

func (s *FillSimulation) openOrders(instID string) []live.OrderStatus {
	s.OKX.mu.Lock()
	defer s.OKX.mu.Unlock()
	out := []live.OrderStatus{}
	for _, order := range s.OKX.orders {
		if order.InstID != instID {
			continue
		}
		if order.Status == live.StatusSubmitted || order.Status == live.StatusPartialFill {
			out = append(out, order)
		}
	}
	return out
}

func conservativeFillQuantity(order live.OrderStatus, low, high float64) float64 {
	remaining := order.Quantity - order.FilledQuantity
	if remaining <= 0 {
		return 0
	}
	span := high - low
	if span <= 0 {
		return remaining * 0.5
	}
	return remaining
}
