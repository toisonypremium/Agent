package storage

import (
	"database/sql"
	"fmt"
	"strings"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
)

// ValidateDCALayerReservationTx applies only to orders with explicit dca_ source
// provenance. It prevents concurrent layers for one thesis and requires every
// prior layer to have a known terminal exchange outcome. It is independent of
// market permission and cap checks, which are enforced before reservation.
func ValidateDCALayerReservationTx(tx *sql.Tx, desired liveguard.ManagedDesiredOrder) error {
	if !isDCAOrderSource(desired.Source) {
		return nil
	}
	if desired.LayerIndex < 1 || desired.LayerIndex > 3 {
		return fmt.Errorf("DCA layer must be 1 through 3")
	}
	var open int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM live_orders WHERE thesis_id=? AND side='BUY' AND source LIKE 'dca_%' AND status IN ('PLANNED','SUBMITTED','LIVE_OPEN','PARTIALLY_FILLED','UNKNOWN_NEEDS_MANUAL_CHECK')`, strings.TrimSpace(desired.ThesisID)).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return fmt.Errorf("DCA thesis already has unresolved layer")
	}
	for prior := 1; prior < desired.LayerIndex; prior++ {
		var status string
		err := tx.QueryRow(`SELECT status FROM live_orders WHERE thesis_id=? AND layer_index=? AND side='BUY' AND source LIKE 'dca_%' ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(desired.ThesisID), prior).Scan(&status)
		if err == sql.ErrNoRows {
			return fmt.Errorf("DCA layer %d requires reconciled terminal layer %d", desired.LayerIndex, prior)
		}
		if err != nil {
			return err
		}
		switch live.NormalizeOrderStatus(status) {
		case live.StatusFilled, live.StatusCancelled, live.StatusRejected, live.StatusExpired:
		default:
			return fmt.Errorf("DCA layer %d prior layer %d not terminal", desired.LayerIndex, prior)
		}
	}
	return nil
}
