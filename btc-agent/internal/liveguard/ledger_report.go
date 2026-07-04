package liveguard

import (
	"fmt"
	"time"

	"btc-agent/internal/exchange/live"
)

type LiveLedgerReport struct {
	GeneratedAt         time.Time                `json:"generated_at"`
	Updated             int                      `json:"updated"`
	Events              []live.LivePositionEvent `json:"events"`
	Positions           []live.LivePosition      `json:"positions"`
	ManualCheckRequired []string                 `json:"manual_check_required"`
	Summary             string                   `json:"summary"`
}

func LiveLedgerSummary(report LiveLedgerReport) string {
	if len(report.Positions) == 0 && report.Updated == 0 && len(report.ManualCheckRequired) == 0 {
		return "no live positions recorded"
	}
	return fmt.Sprintf("live ledger updated %d events, positions %d, manual checks %d", report.Updated, len(report.Positions), len(report.ManualCheckRequired))
}
