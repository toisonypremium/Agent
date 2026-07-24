---
name: trading-long-running
description: Maintain safe long-running BTC Agent work with operator stop controls, steering, durable progress handoffs, and checkpoint-only commits.
---

# Long-running operator controls

These controls supplement [AGENTS.md](../../AGENTS.md). They do not grant
trading, deployment, credential, or database authority.

## Stop control

Before any non-read-only operation, check whether `AGENT_STOP` exists at the
repository root.

```text
AGENT_STOP exists  → stop work immediately.
AGENT_STOP absent  → continue only under existing project rules.
```

Only the operator may remove `AGENT_STOP`. Never remove or overwrite it.

## Steering control

Before resuming a long-running task, inspect `STEER.md` at the repository root.
If it has non-empty content:

1. Treat it as operator guidance.
2. Preserve it as an audit record; do not clear it automatically.
3. Reconcile it with `AGENTS.md` safety precedence.
4. Update the task artifact or handoff note before executing it.

## Handoff convention

For long-running implementation, maintain `PROGRESS.md` at the repository root
with these sections:

```markdown
## Done
## In progress
## Next
## Notes
```

Keep it concise. Record commits, validation evidence, runtime state, and
explicit operator approvals. Do not put credentials, tokens, or private account
data in it.

## Commit boundary

Do not auto-commit on session stop. Commit only at meaningful, validated
checkpoints after reviewing `git diff --check` and `git status`.

## Evidence standard

For Go/storage/execution work, evidence is deterministic temp SQLite tests plus
the project validation suite. Screenshot-only proof is not a substitute.
Production observations remain read-only unless the operator gives explicit,
scoped runtime authorization.
