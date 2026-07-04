package liveguard

import (
	"fmt"
	"math"
	"strings"
	"time"

	"btc-agent/internal/exchange/live"
)

const fillEpsilon = 1e-12

func BuildPositionEvent(previous live.LiveFillSnapshot, status live.OrderStatus, now time.Time) (live.LivePositionEvent, bool, error) {
	if status.Status == live.StatusUnknownNeedsManualCheck {
		return live.LivePositionEvent{}, false, nil
	}

	cumulative := status.AccumulatedFillSz
	if cumulative == 0 {
		cumulative = status.FilledQuantity
	}
	if cumulative <= 0 {
		return live.LivePositionEvent{}, false, nil
	}
	if cumulative+fillEpsilon < previous.FilledQuantity {
		return live.LivePositionEvent{}, false, fmt.Errorf("remote cumulative fill %.12f below local fill %.12f for %s/%s", cumulative, previous.FilledQuantity, status.ClientOrderID, status.OrderID)
	}

	delta := cumulative - previous.FilledQuantity
	if math.Abs(delta) <= fillEpsilon {
		return live.LivePositionEvent{}, false, nil
	}

	fillPrice := status.AvgPrice
	if fillPrice <= 0 {
		fillPrice = status.Price
	}
	if fillPrice <= 0 {
		return live.LivePositionEvent{}, false, fmt.Errorf("filled order %s/%s missing fill price", status.ClientOrderID, status.OrderID)
	}

	feeDelta := status.Fee - previous.Fee
	feeCurrency := strings.ToUpper(status.FeeCurrency)
	if feeCurrency == "" {
		feeCurrency = strings.ToUpper(previous.FeeCurrency)
	}

	symbol := live.InternalSymbol(status.InstID)
	if previous.Symbol != "" {
		symbol = previous.Symbol
	}

	return live.LivePositionEvent{
		Timestamp:     now.Unix(),
		ClientOrderID: status.ClientOrderID,
		OrderID:       status.OrderID,
		InstID:        status.InstID,
		Symbol:        symbol,
		Side:          strings.ToUpper(status.Side),
		DeltaQuantity: delta,
		FillPrice:     fillPrice,
		NotionalDelta: delta * fillPrice,
		FeeDelta:      feeDelta,
		FeeCurrency:   feeCurrency,
		Status:        status.Status,
	}, true, nil
}

func FillSnapshotFromStatus(status live.OrderStatus) live.LiveFillSnapshot {
	filled := status.AccumulatedFillSz
	if filled == 0 {
		filled = status.FilledQuantity
	}
	avgPrice := status.AvgPrice
	if avgPrice == 0 {
		avgPrice = status.Price
	}
	return live.LiveFillSnapshot{
		ClientOrderID:  status.ClientOrderID,
		OrderID:        status.OrderID,
		InstID:         status.InstID,
		Symbol:         live.InternalSymbol(status.InstID),
		Side:           strings.ToUpper(status.Side),
		FilledQuantity: filled,
		AvgPrice:       avgPrice,
		Fee:            status.Fee,
		FeeCurrency:    strings.ToUpper(status.FeeCurrency),
		UpdatedAt:      status.UpdatedAt,
	}
}
