package storage

import (
	"btc-agent/internal/liveguard"
	"database/sql"
	"fmt"
	"strings"
)

// ValidateDCAGlobalCapReservationTx makes DCA exposure cap an atomic
// reservation invariant. Generic/legacy orders are deliberately out of scope.
func ValidateDCAGlobalCapReservationTx(tx *sql.Tx, desired liveguard.ManagedDesiredOrder) error {
	if !isDCAOrderSource(desired.Source) {
		return nil
	}
	var envelope float64
	if err := tx.QueryRow(`SELECT envelope_usdt FROM dca_allocation_epochs ORDER BY id DESC LIMIT 1`).Scan(&envelope); err == sql.ErrNoRows {
		return fmt.Errorf("DCA allocation epoch required")
	} else if err != nil {
		return err
	}
	var cap float64
	err := tx.QueryRow(`SELECT global_cap_percent FROM dca_execution_state WHERE singleton=1`).Scan(&cap)
	if err == sql.ErrNoRows {
		cap = dcaGlobalCapInitial
	} else if err != nil {
		return err
	}
	var used float64
	if err = tx.QueryRow(`SELECT COALESCE(SUM(reserved_usdt+filled_usdt),0) FROM thesis_capital_ledgers`).Scan(&used); err != nil {
		return err
	}
	limit := envelope * cap / 100
	if desired.Notional <= 0 || used+desired.Notional > limit+1e-9 {
		return fmt.Errorf("DCA global exposure cap exceeded: used=%.8f requested=%.8f cap=%.8f", used, desired.Notional, limit)
	}
	return nil
}

func isDCAOrderSource(source string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(source)), "dca_")
}
