package dca

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"btc-agent/internal/okxassets"
	"btc-agent/internal/storage"
)

type ArtifactSource interface {
	Load(now time.Time) (okxassets.Artifact, error)
}
type FileArtifactSource struct {
	Dir    string
	MaxAge time.Duration
}

func (s FileArtifactSource) Load(now time.Time) (okxassets.Artifact, error) {
	return okxassets.LoadArtifact(s.Dir, now, s.MaxAge)
}

type AllocationCoordinator struct {
	DB     *storage.DB
	Source ArtifactSource
	Now    func() time.Time
}
type AllocationCycleResult struct {
	Observed bool
	Proposal *storage.DCAAllocationProposal
	EpochID  int64
	Applied  bool
	Reason   string
}

// ObserveAndMaybeAllocate runs only in the trusted writable scheduler process.
// It accepts only a fresh, verified artifact and has no order/cancel authority.
func (c AllocationCoordinator) ObserveAndMaybeAllocate() (AllocationCycleResult, error) {
	if c.DB == nil || c.Source == nil {
		return AllocationCycleResult{}, fmt.Errorf("allocation coordinator unavailable")
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	a, err := c.Source.Load(now)
	if err != nil {
		return AllocationCycleResult{Reason: "artifact_unavailable"}, nil
	}
	if a.State != okxassets.StateVerified {
		return AllocationCycleResult{Reason: "artifact_not_verified"}, nil
	}
	observedAt, err := time.Parse(time.RFC3339, a.ObservedAt)
	if err != nil {
		return AllocationCycleResult{Reason: "artifact_timestamp_invalid"}, nil
	}
	available, ok := artifactAvailableUSDT(a)
	if !ok {
		return AllocationCycleResult{Reason: "available_usdt_missing"}, nil
	}
	key := "okx-usdt:" + observedAt.UTC().Format(time.RFC3339Nano) + ":" + strconv.FormatFloat(available, 'f', -1, 64)
	if _, err = c.DB.RecordVerifiedUSDTObservation(storage.VerifiedUSDTObservation{ObservationKey: key, AvailableUSDT: available, ObservedAt: observedAt.UTC()}); err != nil {
		return AllocationCycleResult{}, err
	}
	out := AllocationCycleResult{Observed: true}
	proposal, err := c.DB.EvaluateDCAAllocation(now)
	if err != nil {
		return out, err
	}
	if !proposal.Ready {
		out.Reason = proposal.Reason
		return out, nil
	}
	out.Proposal = &proposal
	epochKey := "dca-allocation:" + proposal.ObservedAt.UTC().Format(time.RFC3339Nano) + ":" + strconv.FormatFloat(proposal.NetNewUSDT, 'f', -1, 64)
	epoch, _, err := c.DB.CreateDCAAllocationEpoch(storage.DCAAllocationEpochRequest{IdempotencyKey: epochKey, ObservedAvailableUSDT: proposal.ObservedAvailableUSDT, EnvelopeUSDT: proposal.EnvelopeUSDT, NetNewUSDT: proposal.NetNewUSDT, ObservedAt: proposal.ObservedAt})
	if err != nil {
		return out, err
	}
	out.EpochID = epoch.ID
	applied, err := c.DB.ApplyDCAAllocationEpochToTheses(epoch.ID)
	if err != nil {
		return out, err
	}
	out.Applied = applied
	return out, nil
}
func artifactAvailableUSDT(a okxassets.Artifact) (float64, bool) {
	for _, asset := range a.Assets {
		if strings.EqualFold(asset.Currency, "USDT") {
			v, e := strconv.ParseFloat(asset.Available, 64)
			return v, e == nil && v >= 0
		}
	}
	return 0, false
}
