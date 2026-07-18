# 0001: Atomic reconciled fill is the thesis-capital fill write path

Status: Accepted
Date: 2026-07-18

## Context

A confirmed fill affects positions, immutable events, fill snapshots, thesis
capital, terminal residual reservation, and thesis lifecycle. Independent writes
can leave recovery projections inconsistent after a crash or partial failure.

## Decision

Thesis-aware confirmed fills use `ApplyReconciledLiveFill` as the atomic write
boundary. The transaction applies all required position, fill, capital, release,
and lifecycle projections. Replays are idempotent; deterministic collisions and
provenance conflicts fail and roll back.

## Alternatives considered

- Separate storage calls coordinated by reconcile: rejected because crashes can
  persist partial state.
- Infer thesis from symbol during recovery: rejected because multiple or legacy
  provenance cannot be established safely.
- Auto-repair projection drift: rejected because it can conceal corruption and
  acquire unintended capital authority.

## Safety invariants

- Unknown outcomes never release reservation.
- No thesis identity inference.
- Projection failure rolls back all writes.
- Lifecycle projection cannot submit a SELL.
- Legacy rows remain valid with nullable thesis provenance.

## Consequences

Callers have one deeper interface and storage owns transaction ordering. New
fill-related projections must join this transaction or remain explicitly
read-only. Recovery audits report drift without mutation.

## Validation

Focused replay/collision/rollback tests, full Go tests, race tests, vet, build,
and restart/readback provenance tests.
