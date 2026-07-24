package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	DCAAllocationBootstrap = "BOOTSTRAP"
	DCAAllocationDeposit   = "DEPOSIT"
	allocationRatio        = .80
	minimumNetFunding      = 50.0
	stabilityWindow        = 15 * time.Minute
	stableUSDTTolerance    = .01 // one cent; observer micro-precision is not funding movement
)

type VerifiedUSDTObservation struct {
	ObservationKey string
	AvailableUSDT  float64
	ObservedAt     time.Time
}
type DCAAllocationProposal struct {
	Ready                                           bool
	Kind, Reason                                    string
	ObservedAvailableUSDT, EnvelopeUSDT, NetNewUSDT float64
	ObservedAt                                      time.Time
}

func (d *DB) RecordVerifiedUSDTObservation(o VerifiedUSDTObservation) (bool, error) {
	o.ObservationKey = strings.TrimSpace(o.ObservationKey)
	o.ObservedAt = o.ObservedAt.UTC()
	if o.ObservationKey == "" || o.ObservedAt.IsZero() || math.IsNaN(o.AvailableUSDT) || math.IsInf(o.AvailableUSDT, 0) || o.AvailableUSDT < 0 {
		return false, fmt.Errorf("invalid verified USDT observation")
	}
	payload, err := json.Marshal(o)
	if err != nil {
		return false, err
	}
	var existing string
	err = d.QueryRow(`SELECT payload_json FROM verified_usdt_observations WHERE observation_key=?`, o.ObservationKey).Scan(&existing)
	if err == nil {
		if existing != string(payload) {
			return false, fmt.Errorf("verified USDT observation conflict")
		}
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = d.Exec(`INSERT INTO verified_usdt_observations(observation_key,available_usdt,observed_at,payload_json) VALUES(?,?,?,?)`, o.ObservationKey, o.AvailableUSDT, o.ObservedAt.Unix(), string(payload))
	return err == nil, err
}

func (d *DB) EvaluateDCAAllocation(now time.Time) (DCAAllocationProposal, error) {
	now = now.UTC()
	var latest float64
	var at int64
	err := d.QueryRow(`SELECT available_usdt,observed_at FROM verified_usdt_observations ORDER BY observed_at DESC LIMIT 1`).Scan(&latest, &at)
	if err == sql.ErrNoRows {
		return DCAAllocationProposal{Reason: "no_verified_usdt"}, nil
	}
	if err != nil {
		return DCAAllocationProposal{}, err
	}
	p := DCAAllocationProposal{ObservedAvailableUSDT: latest, ObservedAt: time.Unix(at, 0).UTC()}
	if now.Before(p.ObservedAt) {
		p.Reason = "observation_from_future"
		return p, nil
	}
	var allocated float64
	var epochCount int
	if err := d.QueryRow(`SELECT COUNT(*),COALESCE(SUM(net_new_usdt),0) FROM dca_allocation_epochs`).Scan(&epochCount, &allocated); err != nil {
		return p, err
	}
	baseline := allocated / allocationRatio
	if epochCount > 0 && latest < baseline-stableUSDTTolerance {
		p.Reason = "verified_usdt_decreased"
		return p, nil
	}
	var count int
	var first int64
	if err := d.QueryRow(`SELECT COUNT(*),MIN(observed_at) FROM verified_usdt_observations WHERE ABS(available_usdt-?)<=? AND observed_at>=? AND observed_at<=?`, latest, stableUSDTTolerance, p.ObservedAt.Add(-stabilityWindow).Unix(), p.ObservedAt.Unix()).Scan(&count, &first); err != nil {
		return p, err
	}
	if count < 2 || p.ObservedAt.Sub(time.Unix(first, 0).UTC()) < stabilityWindow {
		p.Reason = "funding_not_stable"
		return p, nil
	}
	if epochCount == 0 {
		p.Kind = DCAAllocationBootstrap
		p.NetNewUSDT = latest * allocationRatio
		p.EnvelopeUSDT = p.NetNewUSDT
		p.Ready = true
		return p, nil
	}
	p.NetNewUSDT = (latest - baseline) * allocationRatio
	p.EnvelopeUSDT = p.NetNewUSDT
	if p.NetNewUSDT < minimumNetFunding {
		p.Reason = "net_funding_below_minimum"
		return p, nil
	}
	p.Kind = DCAAllocationDeposit
	p.Ready = true
	return p, nil
}
