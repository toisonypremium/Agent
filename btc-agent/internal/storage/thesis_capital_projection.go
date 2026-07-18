package storage

import (
	"fmt"
	"math"
)

type ThesisCapitalProjection struct {
	Ledger                    ThesisCapitalLedger
	JournalFilledUSDT         float64
	JournalReleasedUSDT       float64
	ActiveReservedUSDT        float64
	ProjectedFilledUSDT       float64
	ProjectedReservedUSDT     float64
	ProjectedRemainingDCAUSDT float64
	Mismatches                []string
}

func (p ThesisCapitalProjection) Healthy() bool { return len(p.Mismatches) == 0 }

// ThesisCapitalProjectionAudit is read-only. It never repairs ledger state.
func (d *DB) ThesisCapitalProjectionAudit(thesisID string) (ThesisCapitalProjection, error) {
	ledger, err := d.ThesisCapitalLedgerByID(thesisID)
	if err != nil {
		return ThesisCapitalProjection{}, err
	}
	p := ThesisCapitalProjection{Ledger: ledger}
	if err := d.QueryRow(`SELECT COALESCE(SUM(CASE WHEN event_type=? THEN notional_usdt ELSE 0 END),0), COALESCE(SUM(CASE WHEN event_type=? THEN notional_usdt ELSE 0 END),0) FROM thesis_capital_events WHERE thesis_id=?`, ThesisCapitalEventBuyFill, ThesisCapitalEventRelease, ledger.ThesisID).Scan(&p.JournalFilledUSDT, &p.JournalReleasedUSDT); err != nil {
		return ThesisCapitalProjection{}, err
	}
	if err := d.QueryRow(`SELECT COALESCE(SUM(o.notional-COALESCE((SELECT SUM(e.notional_usdt) FROM thesis_capital_events e WHERE e.client_order_id=o.client_order_id AND e.event_type=?),0)),0) FROM live_orders o WHERE o.thesis_id=? AND UPPER(o.side)='BUY' AND o.status IN ('PLANNED','SUBMITTED','PARTIAL_FILL','LIVE_OPEN','PARTIALLY_FILLED','UNKNOWN_NEEDS_MANUAL_CHECK')`, ThesisCapitalEventBuyFill, ledger.ThesisID).Scan(&p.ActiveReservedUSDT); err != nil {
		return ThesisCapitalProjection{}, err
	}
	p.ProjectedFilledUSDT = p.JournalFilledUSDT
	p.ProjectedReservedUSDT = p.ActiveReservedUSDT
	if p.ProjectedReservedUSDT < 0 && p.ProjectedReservedUSDT > -1e-9 {
		p.ProjectedReservedUSDT = 0
	}
	p.ProjectedRemainingDCAUSDT = ledger.MaxExposureUSDT - p.ProjectedFilledUSDT - p.ProjectedReservedUSDT
	if math.Abs(ledger.FilledUSDT-p.ProjectedFilledUSDT) > 1e-8 {
		p.Mismatches = append(p.Mismatches, fmt.Sprintf("filled ledger=%.8f projected=%.8f", ledger.FilledUSDT, p.ProjectedFilledUSDT))
	}
	if math.Abs(ledger.ReservedUSDT-p.ProjectedReservedUSDT) > 1e-8 {
		p.Mismatches = append(p.Mismatches, fmt.Sprintf("reserved ledger=%.8f projected=%.8f", ledger.ReservedUSDT, p.ProjectedReservedUSDT))
	}
	if math.Abs(ledger.RemainingDCAUSDT-p.ProjectedRemainingDCAUSDT) > 1e-8 {
		p.Mismatches = append(p.Mismatches, fmt.Sprintf("remaining DCA ledger=%.8f projected=%.8f", ledger.RemainingDCAUSDT, p.ProjectedRemainingDCAUSDT))
	}
	if p.ProjectedReservedUSDT < -1e-8 || p.ProjectedFilledUSDT < -1e-8 || p.ProjectedRemainingDCAUSDT < -1e-8 {
		p.Mismatches = append(p.Mismatches, "projected capital became negative")
	}
	return p, nil
}

func (d *DB) ThesisCapitalProjectionAudits() ([]ThesisCapitalProjection, error) {
	rows, err := d.Query(`SELECT thesis_id FROM thesis_capital_ledgers ORDER BY thesis_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ThesisCapitalProjection{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		p, err := d.ThesisCapitalProjectionAudit(id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
