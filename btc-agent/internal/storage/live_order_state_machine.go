package storage

import (
	"fmt"
	"strings"

	"btc-agent/internal/exchange/live"
)

func validLiveOrderTransition(from, to string) bool {
	from = live.NormalizeOrderStatus(strings.TrimSpace(from))
	to = live.NormalizeOrderStatus(strings.TrimSpace(to))
	if from == to {
		return true
	}
	switch from {
	case live.StatusPlanned:
		return to == live.StatusSubmitted || to == live.StatusPartialFill || to == live.StatusFilled || to == live.StatusRejected || to == live.StatusCancelled || to == live.StatusUnknownNeedsManualCheck || to == live.StatusExpired
	case live.StatusSubmitted:
		return to == live.StatusPartialFill || to == live.StatusFilled || to == live.StatusRejected || to == live.StatusCancelled || to == live.StatusUnknownNeedsManualCheck || to == live.StatusExpired
	case live.StatusPartialFill:
		return to == live.StatusPartialFill || to == live.StatusFilled || to == live.StatusCancelled || to == live.StatusUnknownNeedsManualCheck || to == live.StatusExpired
	case live.StatusUnknownNeedsManualCheck:
		return to == live.StatusSubmitted || to == live.StatusPartialFill || to == live.StatusFilled || to == live.StatusCancelled || to == live.StatusRejected || to == live.StatusExpired || to == live.StatusUnknownNeedsManualCheck
	case live.StatusFilled, live.StatusCancelled, live.StatusRejected, live.StatusExpired:
		return false
	default:
		return false
	}
}

func ensureLiveOrderTransition(current, next string) error {
	if !validLiveOrderTransition(current, next) {
		return fmt.Errorf("invalid live order state transition: %s -> %s", current, next)
	}
	return nil
}
