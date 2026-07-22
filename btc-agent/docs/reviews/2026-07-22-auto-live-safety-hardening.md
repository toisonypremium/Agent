# Codex Review Handoff: Auto-Live Safety Hardening

## Scope

- Correct `.gitignore` so the local `config.yaml` is ignored.
- Require `live_auto_mode`, supervisor, and managed order control at config,
  CLI runtime, and final managed-execution assertion boundaries.
- Add focused regression tests for every required runtime control.
- Add an operator runbook for preflight, operator halt, and rollback.

## Production evidence

- No production configuration, environment file, database, scheduler, service,
  or credential was changed.
- No exchange call, order placement, cancellation, or live scheduler start was
  performed.
- Isolated preflight used `.goal-check/config.yaml`, copied from the safe example.
  It failed closed because no plan exists in that empty database.

## Tests

- `go test ./...`
- `go vet ./...`
- `go build -o bin/btc-agent .`
- `git diff --check`
- `go test ./internal/liveguard ./internal/config .`

## High-risk files

- `cmd_live.go`: process-level auto-live gate.
- `internal/config/config.go`: auto-live configuration validation.
- `internal/liveguard/execution_assertion.go`: final per-order assertion.
- `scripts/btc-agent-scheduler.sh`: real scheduler wrapper; reviewed only.

## Mandatory review questions

1. Can any order-placing entrypoint bypass `requireAutoLiveRuntime` and the
   managed final assertion?
2. Does adding `live_auto_mode`, supervisor, and order-management checks break
   a legitimate non-automatic manual workflow?
3. Are dry-run paths still side-effect free with regard to exchange mutations?
4. Is the runbook clear that `APPROVED_REAL_ORDER` requires fresh market,
   reconciliation, proof, and operator evidence rather than configuration alone?

## Safety boundaries

- Spot post-only limit BUY only.
- No futures, leverage, market order, automatic sell, or automatic resume.
- Missing or stale evidence must block execution.
- Dashboard and Telegram remain read-only.

## Rollback

Revert only the intentional source and test changes. Do not reset or discard the
pre-existing dirty worktree. If a live process is active, use `operator-halt`
before any code rollback and preserve reports/database for review.
