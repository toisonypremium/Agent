package storage

import (
	"fmt"
	"math"
)

type PortfolioCapitalInvariantReport struct {
	ConfiguredInvestableEnvelopeUSDT float64                   `json:"configured_investable_envelope_usdt"`
	ThesisMaxExposureUSDT            float64                   `json:"thesis_max_exposure_usdt"`
	FilledUSDT                       float64                   `json:"filled_usdt"`
	ReservedUSDT                     float64                   `json:"reserved_usdt"`
	RemainingDCAUSDT                 float64                   `json:"remaining_dca_usdt"`
	ActiveOrderNetReservedUSDT       float64                   `json:"active_order_net_reserved_usdt"`
	ThesisCount                      int                       `json:"thesis_count"`
	DriftedThesisCount               int                       `json:"drifted_thesis_count"`
	OrphanThesisOrderCount           int                       `json:"orphan_thesis_order_count"`
	OrphanCapitalEventCount          int                       `json:"orphan_capital_event_count"`
	EnvelopeExcessUSDT               float64                   `json:"envelope_excess_usdt"`
	Healthy                          bool                      `json:"healthy"`
	Issues                           []string                  `json:"issues,omitempty"`
	Theses                           []ThesisCapitalProjection `json:"theses"`
}

// PortfolioCapitalInvariantAudit is read-only. The envelope is supplied by the
// caller's planning/config layer and is never treated as reservation authority.
func (d *DB) PortfolioCapitalInvariantAudit(configuredInvestableEnvelopeUSDT float64) (PortfolioCapitalInvariantReport, error) {
	if configuredInvestableEnvelopeUSDT < 0 || math.IsNaN(configuredInvestableEnvelopeUSDT) || math.IsInf(configuredInvestableEnvelopeUSDT, 0) {
		return PortfolioCapitalInvariantReport{}, fmt.Errorf("configured investable envelope must be finite and non-negative")
	}
	r := PortfolioCapitalInvariantReport{ConfiguredInvestableEnvelopeUSDT: configuredInvestableEnvelopeUSDT, Healthy: true}
	projections, err := d.ThesisCapitalProjectionAudits()
	if err != nil {
		return r, err
	}
	r.Theses = projections
	r.ThesisCount = len(projections)
	for _, p := range projections {
		r.ThesisMaxExposureUSDT += p.Ledger.MaxExposureUSDT
		r.FilledUSDT += p.Ledger.FilledUSDT
		r.ReservedUSDT += p.Ledger.ReservedUSDT
		r.RemainingDCAUSDT += p.Ledger.RemainingDCAUSDT
		r.ActiveOrderNetReservedUSDT += p.ActiveReservedUSDT
		if !p.Healthy() {
			r.DriftedThesisCount++
		}
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM live_orders o WHERE COALESCE(o.thesis_id,'')<>'' AND NOT EXISTS(SELECT 1 FROM thesis_capital_ledgers l WHERE l.thesis_id=o.thesis_id)`).Scan(&r.OrphanThesisOrderCount); err != nil {
		return r, err
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM thesis_capital_events e WHERE NOT EXISTS(SELECT 1 FROM thesis_capital_ledgers l WHERE l.thesis_id=e.thesis_id) OR NOT EXISTS(SELECT 1 FROM live_orders o WHERE o.client_order_id=e.client_order_id)`).Scan(&r.OrphanCapitalEventCount); err != nil {
		return r, err
	}
	if r.ThesisMaxExposureUSDT > configuredInvestableEnvelopeUSDT+1e-8 {
		r.EnvelopeExcessUSDT = r.ThesisMaxExposureUSDT - configuredInvestableEnvelopeUSDT
		r.Issues = append(r.Issues, fmt.Sprintf("thesis max exposure exceeds configured investable envelope by %.8f", r.EnvelopeExcessUSDT))
	}
	if r.DriftedThesisCount > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d thesis capital projections drifted", r.DriftedThesisCount))
	}
	if r.OrphanThesisOrderCount > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d thesis orders have no ledger", r.OrphanThesisOrderCount))
	}
	if r.OrphanCapitalEventCount > 0 {
		r.Issues = append(r.Issues, fmt.Sprintf("%d capital events are orphaned", r.OrphanCapitalEventCount))
	}
	if math.Abs(r.ReservedUSDT-r.ActiveOrderNetReservedUSDT) > 1e-8 {
		r.Issues = append(r.Issues, fmt.Sprintf("portfolio reserved ledger=%.8f active orders=%.8f", r.ReservedUSDT, r.ActiveOrderNetReservedUSDT))
	}
	r.Healthy = len(r.Issues) == 0
	return r, nil
}
