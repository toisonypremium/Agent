# Unattended Live Readiness

## Deployment profile

Use the immutable unprivileged user service only: `deploy/systemd/btc-agent-immutable.service` with `$HOME/btc-agent/immutable/current` and `$HOME/btc-agent/runtime`. Install with `deploy/systemd/install-immutable-user-service.sh` and verify with `deploy/verify-immutable-runtime.sh`.

## Authority model

```text
$HOME/btc-agent/runtime/config.yaml    configuration (mode 0600)
$HOME/btc-agent-systemd.env            protected service environment
$HOME/btc-agent/runtime                SQLite, reports, runtime state
$HOME/btc-agent/runtime/backups        verified snapshots
$HOME/btc-agent/immutable/releases     immutable releases
$HOME/btc-agent/immutable/current      atomic active-release symlink
```

Cron, PM2, legacy service units, Termux boot loops, and shell scheduler wrappers must not launch a second scheduler. The immutable user service is the sole authority. Do not clear operator halt as part of deployment or restart.

## Pre-release gate

1. Worktree is clean and the exact Git SHA is reviewed.
2. CI passes format, static analysis, vulnerability scan, tests, race tests, Linux build, immutable runtime drills and secret scan.
3. Build the binary outside production and record SHA-256.
4. Keep operator halt active.
5. Run `$HOME/btc-agent/immutable/backup.sh`, then `$HOME/btc-agent/immutable/verify-backup.sh <archive>`.
6. Confirm exactly one immutable scheduler is enabled and no legacy loop, cron, PM2 entry, or service can run.
7. Retain the preceding immutable release and its verified backup.

## Shadow verification

With the protected service environment loaded, run the commands in
[production-verification-checklist.md](production-verification-checklist.md).
Record UTC start/end, Git SHA, binary SHA, owner instance, fencing token, lease
freshness, operator halt, reconciliation, alerts and
dry-run exchange call count.

Observe for at least seven days with halt active. Exercise a controlled service
restart and a reboot. A stale lease or heartbeat, unknown/remote-only order,
reconcile mismatch, dead-letter growth, failed alert, balance drift, or duplicate
scheduler fails the window. Preserve evidence; restart the window after a failure.

## Canary and live authority

Only an explicit authorized operator can clear halt. `APPROVED_MONITORING` and
`APPROVED_DRY_RUN` are not execution permission. Before any canary, require
`APPROVED_CANARY`, clean current reconciliation, fresh ownership, all market gates,
first-order quarantine and a separately recorded order limit. Never enable
unrestricted autonomous trading from this runbook.

## Incident actions

For uncertain exchange outcome, stale safety signal, security concern, or ownership
loss: request/activate operator halt, preserve DB/WAL/logs, snapshot remote
balances/positions/orders, and reconcile before retrying. Never send a replacement
order with a new client ID until the original outcome is known.
