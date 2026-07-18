---
name: trading-tdd
description: Test-driven development for BTC Agent features and fixes using Go public seams, deterministic SQLite fixtures, and trading-safety regression cases. Use for test-first work, red-green-refactor, integration tests, replay, or rollback coverage.
---

# Trading-safe TDD

Read `CONTEXT.md`, relevant ADRs, and `.agents/AGENTS.md` first.

## Public seams

Prefer these existing seams unless the task establishes a better one:

- storage transaction APIs;
- managed-order reservation interface;
- reconciled-fill application boundary;
- lifecycle evaluator public API;
- opsplan/report boundary;
- config validation boundary.

Test observable behavior, persisted state, returned errors, and authority. Do
not assert private helper shape. Use temp SQLite DBs and deterministic IDs/time.

## Red-green-refactor

1. Write one test for one capability; run it and prove the expected failure.
2. Implement the smallest behavior that passes.
3. Run the focused test twice.
4. Refactor without changing behavior.
5. Extend the matrix only for distinct risk classes.

For capital/order/reconcile changes cover applicable cases:

```text
success
idempotent replay
deterministic collision
transaction rollback
legacy compatibility
unknown-outcome fail-closed
provenance conflict
restart/readback
```

Use tolerances for floating-point comparisons. Mocks must not reproduce the
implementation; prefer stateful fakes at exchange or storage seams. Never use
production DB, credentials, or real orders.

Before completion run full tests, vet, build, diff check, and relevant race
tests.
