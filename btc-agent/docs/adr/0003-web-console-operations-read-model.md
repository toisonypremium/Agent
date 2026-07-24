# 0003: Operations read models and capital allocation remain observation-only

Status: Accepted

Date: 2026-07-24

## Context

The Web Console has an authenticated loopback-origin deployment and a narrow
one-way halt request. Operators need richer visibility of runtime health, paper
evidence, market/planner context, capital posture, incidents, and approved
reports without allowing a browser to acquire runtime, capital, or execution
authority.

Capital data is particularly sensitive. A thesis is explicit investment
provenance and must never be inferred from a symbol. Reservations, reconciled
fills, releases, lifecycle, and planning have separate owners. An unknown
exchange outcome keeps capital locked.

## Decision

Build the expanded console as typed, read-first operations modules. Each module
uses explicit allowlisted DTOs, provenance timestamps, and
fresh/stale/unavailable semantics.

### Runtime health

Use an observer-owned, persisted typed snapshot derived from existing heartbeat
and verifier evidence. The Web Console reads that snapshot; it never invokes
shell commands, `systemctl`, filesystem discovery, a release installer, or a
scheduler control operation.

### Capital and allocation

The first capital scope is **USDT within the existing runtime scope only**.

- Read account/capital audits, thesis capital ledgers, reservations and
  reconciled-fill projections only through typed read models.
- `thesis_id` is the only capital/provenance primary key. `symbol` is a display
  reference and must never select, merge, or backfill a thesis.
- The browser may calculate local, non-persistent what-if allocation outcomes
  from a frozen read snapshot. It has no capital/allocation mutation endpoint.
- No browser path writes an envelope, risk limit, reservation, release, capital
  journal entry, planner allocation, exchange balance, or runtime configuration.
- Multi-asset aggregation, FX conversion, and untyped exchange-balance
  reconstruction are excluded.

### Access and audit defaults

- The account owner is the initial `operator` identity; other authenticated
  identities default to `viewer`.
- An `auditor` role can access approved exports and audit identity material only
  when explicitly mapped at the identity boundary.
- A `viewer` sees masked actor identity in audit views.
- The backend must validate the resolved capability before accepting a halt;
  Cloudflare Access alone is not sufficient authorization for mutation.
- Initial report catalog is empty. A report is exposed only after a separate
  allowlist decision records its fixed ID, filename, type, size cap and renderer.

## Alternatives considered

### Make the console a capital allocation editor

Rejected. Browser-configured allocation could cross planning, reservation and
execution boundaries, expose replay/race conditions, and obscure provenance.

### Use symbols as capital identity

Rejected. It violates immutable thesis provenance and creates unsafe ambiguity
for legacy or overlapping records.

### Let Web Console execute operational probes

Rejected. It expands its process authority, weakens sandboxing, and makes UI
availability or parsing a runtime-control dependency.

## Consequences

The console may provide high-signal explanation and local simulation but cannot
make capital effective. Uninstrumented data is rendered as unavailable rather
than inferred. New read fields require a source map and deterministic temporary
SQLite fixtures. Runtime-health collection is an observer concern, not a browser
or scheduler authority change.

## Validation

- DTO tests prove no symbol-to-thesis inference and preserve unknown-outcome
  locked capital.
- Aggregate/audit drift is visible, never repaired.
- Local simulation has no network request or persisted side effect.
- Access tests prove viewer halt rejection and operator-only acceptance.
- Runtime health tests cover fresh, stale, missing and malformed snapshots.
- Full backend/frontend/release checks run before a separate Web release.
