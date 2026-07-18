package storage

import (
	"testing"

	"btc-agent/internal/exchange/live"
)

func TestValidLiveOrderTransitionMatrix(t *testing.T) {
	allowed := [][2]string{
		{live.StatusPlanned, live.StatusSubmitted}, {live.StatusPlanned, live.StatusUnknownNeedsManualCheck},
		{live.StatusSubmitted, live.StatusPartialFill}, {live.StatusSubmitted, live.StatusFilled},
		{live.StatusPartialFill, live.StatusFilled}, {live.StatusPartialFill, live.StatusCancelled},
		{live.StatusUnknownNeedsManualCheck, live.StatusSubmitted}, {live.StatusUnknownNeedsManualCheck, live.StatusFilled},
	}
	for _, pair := range allowed {
		if err := ensureLiveOrderTransition(pair[0], pair[1]); err != nil {
			t.Errorf("allowed %s -> %s: %v", pair[0], pair[1], err)
		}
	}
	blocked := [][2]string{
		{live.StatusFilled, live.StatusSubmitted}, {live.StatusFilled, live.StatusPartialFill},
		{live.StatusCancelled, live.StatusSubmitted}, {live.StatusRejected, live.StatusPlanned},
		{live.StatusUnknownNeedsManualCheck, live.StatusPlanned}, {live.StatusFilled, live.StatusCancelled},
	}
	for _, pair := range blocked {
		if err := ensureLiveOrderTransition(pair[0], pair[1]); err == nil {
			t.Errorf("blocked %s -> %s accepted", pair[0], pair[1])
		}
	}
}

func TestLiveOrderTransitionIsIdempotent(t *testing.T) {
	for _, status := range []string{live.StatusPlanned, live.StatusSubmitted, live.StatusPartialFill, live.StatusFilled, live.StatusUnknownNeedsManualCheck} {
		if err := ensureLiveOrderTransition(status, status); err != nil {
			t.Errorf("same state %s: %v", status, err)
		}
	}
}
