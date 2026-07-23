# BTC Agent Release Runbook

## Preconditions

- Worktree is clean and reviewed.
- Tests, vet, and operational drills (`deploy/test-backup.sh`, `deploy/test-health-check.sh`, `deploy/test-service-unit.sh`) pass.
- Current market authority and operator status are checked.
- A production backup exists outside the Git root and passes `deploy/verify-backup.sh`.
- The production unit is `agent.service`, running an immutable release from `/opt/agent/current`.
- No V1 service, user unit, PM2 entry, cron entry, or Termux boot loop can start another scheduler.
- Never commit runtime config, environment files, databases, logs, reports, or binaries.

## Build

Run from `btc-agent/`:

    VERSION=v0.1.0 scripts/build-release.sh
    bin/btc-agent version
    cat bin/btc-agent.manifest
    cat bin/btc-agent.sha256

The script runs tests and vet, builds to a temporary path, atomically replaces the target binary, and emits checksum and manifest files.

## Deploy

1. Verify the expected commit and checksum.
2. Run `deploy/backup.sh` and `deploy/verify-backup.sh <archive>`.
3. Verify the previous immutable release remains available for rollback.
4. Install through `AGENT_RELEASE_APPROVED=1 deploy/install-release.sh <release-id> <binary-path>`.
5. Confirm service state, PID, process start time, binary version, one scheduler process and no legacy scheduler through `deploy/verify-runtime.sh`.
6. Run `live-doctor`, `live-auto-audit`, and `scheduler-heartbeat-check` with the production environment loaded.
7. Run SQLite `PRAGMA quick_check` and reconciliation.
8. Confirm no unexpected open orders and that trading gates, including operator halt, remain unchanged.

## Rollback

Restore the previous immutable release with `deploy/rollback.sh`, retain the failed
release and its state for investigation, then repeat backup verification, health,
reconciliation and ownership checks. Do not alter trading configuration or clear halt
during a binary rollback.
