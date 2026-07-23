# Agent completion rules

Do not report `done` without file and command evidence.

## Required evidence

- Changed files with concise purpose.
- Exact commands run and PASS/FAIL result.
- Safety-relevant failures, warnings, and validation gaps.
- Remaining risk, blocker, or time-bound evidence requirement.
- Explicit statement when no production action occurred.

## Baseline gate

```bash
make verify
```

CI additionally runs Linux race tests, `staticcheck`, and `govulncheck`.
Local Termux must state the race-test limitation rather than pretending to run
it.

## Risk-based extensions

| Change | Additional proof |
|---|---|
| Storage/execution/reconcile/capital/lifecycle | focused matrix: success, replay, collision, rollback, legacy compatibility, unknown-outcome fail-closed, restart/readback |
| Deployment | approved SHA, release installer result, immutable runtime verifier, halt state, scheduler count, backup evidence |
| Config/credentials/authority | explicit operator approval immediately before action; redacted evidence only |
| Docs-only | links/check commands accurate; no claim of runtime state from docs |

## Hard rules

- If no tracked source, test, deployment, or docs file changed, say so plainly;
  never fabricate completion.
- Do not commit secrets, runtime config, databases, reports, logs, backups, or
  binaries.
- Do not enable autonomous real trading, weaken safety gates, or place real
  orders during development or validation.
- Never claim a runtime operation without command output from that runtime.
