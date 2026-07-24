# BTC Agent project rules

## Safety precedence

These rules override all project skills and upstream-derived guidance.

- Spot-only and DCA-only.
- No futures, leverage, shorting, market BUY, or automatic stop-loss SELL.
- Loss and thesis invalidation may block BUY but never authorize automatic SELL.
- Never resume automatically after operator halt.
- Never infer thesis identity from symbol.
- Never release capital for an unknown exchange outcome.
- Never auto-repair capital ledger or projection drift.
- Never test against or mutate the production DB.
- Never place or cancel a real order during development or validation.
- Never deploy, restart services, change credentials, or enable live execution
  without explicit operator authorization.

## Engineering validation

Use deterministic temp DB fixtures. For storage/execution changes, test success,
replay, collision, rollback, legacy compatibility, and fail-closed outcomes.
Before an isolated commit, run focused tests, `go test ./...`, `go vet ./...`,
`go build`, `git diff --check`, and relevant race tests.

## Long-running work controls

For long-running tasks, follow the `trading-long-running` skill.

- Stop immediately if repository-root `AGENT_STOP` exists; never remove it.
- Treat non-empty repository-root `STEER.md` as operator guidance; preserve it.
- Maintain `PROGRESS.md` only for long-running handoff state, without secrets.
- Never auto-commit on session stop; commit only validated checkpoints.
