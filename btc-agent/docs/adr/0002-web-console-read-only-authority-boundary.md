# 0002: Web Console is read-first and halt-only

Status: Accepted

Date: 2026-07-23

## Context

The historical browser dashboard, cloud read model, and related deployment paths
were intentionally removed. Operators still need a concise visual view of the
immutable, halted-paper runtime, market analysis, planner state, paper evidence,
and audit events.

A Web Console must not blur reporting, planning, storage, reconciliation, and
execution authority. In particular, a browser must not become a route for real
orders, cancellations, configuration changes, or clearing the operator halt.

## Decision

Build a new Web Console as a separate, loopback/private-network process with a
narrow typed Go read-model API.

- All Web Console endpoints are read-only except a single typed halt-request
  endpoint.
- The halt endpoint validates an authenticated operator role, CSRF protection,
  existing reason codes, an idempotency key, and writes an audit receipt before
  delegating only to the existing halt boundary.
- The console has no resume/unhalt route and no generic RPC, CLI passthrough,
  shell execution, config mutation, order submission, order cancellation, or
  reconciliation mutation route.
- The browser receives only allowlisted DTOs and allowlisted reports. It never
  receives raw config, environment values, credentials, direct SQLite access,
  or arbitrary filesystem data.
- The scheduler retains its existing runtime and execution authority. Restarting
  or failing the console cannot restart the scheduler or change execution mode.
- Non-loopback access requires an identity-aware TLS gateway. Authentication and
  role enforcement are server-side.

## Alternatives considered

### Restore the removed dashboard and cloud data model

Rejected. It would restore purged Supabase/R2/browser data paths and their
security/operational surface. It also weakens the clean-slate audit boundary.

### Static report site

Rejected. It does not provide authenticated operations visibility, freshness
semantics, access control, or a safe audited halt-request path.

### Embed a server in the scheduler/execution process

Rejected. It combines Web availability and browser-facing parsing with the
trading runtime, increasing blast radius and making process-level least
privilege impossible.

## Consequences

The first delivery increment is a fixture-backed, read-only visual prototype
and a threat model. Runtime deployment is deferred and requires separate
approval.

New API and data-model work must use typed, allowlisted DTOs. A read-model
module may observe/report/block and forward a narrowly validated halt request;
it cannot obtain exchange, capital, or SELL authority.

## Safety invariants

- Spot/DCA-only constraints are unchanged.
- No market BUY, automatic stop-loss SELL, futures, leverage, or shorting.
- Operator halt never resumes automatically or through the Web Console.
- Unknown exchange outcomes never release capital.
- Web reports do not repair storage, infer thesis provenance, or grant order
  authority.
- Tests use temporary databases and never call a real exchange.

## Validation

Before a production Web Console release:

- typed API, redaction, report-catalog, stale-state, authorization, CSRF,
  idempotency, replay and persistence-failure tests pass;
- no endpoint can invoke order placement, cancellation, resume, config mutation
  or shell execution;
- frontend accessibility, responsive visual, security-header and authenticated
  end-to-end checks pass;
- Linux race/static/vulnerability/build and immutable release verification pass.
