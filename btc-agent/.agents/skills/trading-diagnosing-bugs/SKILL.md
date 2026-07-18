---
name: trading-diagnosing-bugs
description: Evidence-first diagnosis for BTC Agent bugs, deadlocks, reconciliation failures, capital drift, races, and performance regressions. Use when debugging broken, failing, hanging, inconsistent, or slow behavior in this Go trading system.
---

# Trading-safe bug diagnosis

Read `CONTEXT.md`, relevant ADRs, and `.agents/AGENTS.md` first.

## 1. Build a red-capable feedback loop

Name one deterministic command that exercises the exact symptom and can fail
before the fix and pass afterward. Prefer, in order:

1. focused Go regression test through a public interface;
2. SQLite temp DB fixture;
3. captured event/fill replay with sanitized data;
4. rollback assertion after injected failure;
5. `go test -race` stress loop;
6. differential old/new projection or output;
7. isolated mock harness.

Never reproduce against the production DB or live exchange. If no safe loop can
be built, stop and state which sanitized artifact or access is missing.

## 2. Observe before hypothesizing

Capture exact error, inputs, state transition, transaction boundary, and timing.
Separate facts from hypotheses. Check idempotency keys, persisted provenance,
terminal status, connection limits, row lifetimes, and error handling.

## 3. Test hypotheses narrowly

Change one variable at a time. Prefer instrumentation or assertions over broad
patches. For nondeterminism, raise reproduction rate with repeated/race tests,
pinned time, controlled scheduling, and isolated filesystem state.

## 4. Fix the smallest cause

Preserve existing safety and authority boundaries. Unknown exchange outcomes,
operator halt, and capital drift remain fail-closed. Add a regression test that
would fail if the bug returned.

## 5. Validate

Run focused tests twice, then full tests, vet, build, diff check, and relevant
race tests. Report root cause, evidence, fix, regression test, and residual risk.
