# BTC Agent Release Runbook

## Preconditions

- Worktree is clean and reviewed.
- Tests and vet pass.
- Current market authority and operator status are checked.
- A production backup exists outside the Git root.
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
2. Back up the current production binary and database.
3. Replace `bin/btc-agent` atomically.
4. Restart `btc-agent-scheduler.service`.
5. Confirm service state, PID, process start time, and binary version.
6. Run `live-doctor`, `live-auto-audit`, and `scheduler-heartbeat-check` with the production environment loaded.
7. Run SQLite `PRAGMA quick_check`.
8. Confirm no unexpected open orders and that trading gates remain unchanged.

## Rollback

Restore the backed-up binary atomically, restart the service, and repeat every post-deploy check. Do not alter trading configuration during a binary rollback.
