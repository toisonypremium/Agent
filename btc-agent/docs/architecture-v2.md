# Architecture V2 — btc-agent

## Runtime boundary

One Go binary on the VPS is the only execution owner. Vercel serves the dashboard;
Supabase, R2 and Telegram stay outside the critical order path.

```text
Market data -> Domain decision -> Application risk/plan -> Ownership check
-> OKX execution -> SQLite ledger/outbox -> async Supabase/R2/Telegram sync
```

## Target modules

- `cmd/agent`: process entrypoint and dependency wiring.
- `internal/domain`: market, strategy, portfolio, risk, orders and execution rules.
- `internal/application`: analysis, planning, trading, reconciliation, reporting,
  commands.
- `internal/adapters`: OKX, SQLite, Supabase, R2, Telegram and LLM implementations.
- `internal/runtime`: scheduler, ownership, heartbeat, health, retry and outbox.
- `web`: read-only Next.js dashboard; no exchange secrets or direct OKX access.
- `supabase`: versioned migrations, RLS, seed and retention.
- `deploy`: release-based VPS scripts and hardened systemd service.

## Dependency rules

Domain imports no infrastructure, environment, HTTP client, Telegram, filesystem,
Supabase, R2 or SQLite driver. Application depends on ports. Adapters implement
ports. Runtime composes and supervises use cases.

## Safety invariants

- One execution owner; lease expiry or fencing mismatch blocks submission.
- Every order has client ID, idempotency key, correlation ID, decision/plan/instance
  IDs, lifecycle state and audit event.
- Reconciliation with OKX precedes execution.
- `RUN_MODE=paper|shadow|live`; execution is disabled by default.
- Supabase/R2/Vercel/Telegram outages do not stop protection or reconciliation.
- Dashboard stays read-only until approval, challenge and audit are complete.
- V1 remains until shadow comparison, backup and rollback pass.

## Migration order

1. Characterization tests and behavior baseline.
2. Application ports and composition-root extraction.
3. Ownership, fencing, heartbeat, health and durable outbox.
4. Supabase read model and R2 artifact adapter.
5. Read-only dashboard and CI.
6. Release-based deploy/rollback.
7. Shadow comparison, reconcile-only cutover, then V1 cleanup.
