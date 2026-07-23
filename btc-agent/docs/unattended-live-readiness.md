# Unattended Live Readiness

## Deployment profiles

Use one profile only:

- **Root-managed Linux:** `deploy/systemd/agent.service` with `/opt/agent/current`,
  `/etc/agent`, and `/var/lib/agent`.
- **Unprivileged VPS:** `deploy/systemd/btc-agent-immutable.service` with
  `$HOME/btc-agent/immutable/current` and `$HOME/btc-agent/runtime`. Install with
  `deploy/systemd/install-immutable-user-service.sh` and verify with
  `deploy/systemd/verify-immutable-user-service.sh`.

Both profiles run an immutable binary, isolate runtime state from the Git checkout,
and require an active operator halt during shadow observation. Do not run both.

## Authority model

```text
/etc/agent/config.yaml        configuration (mode 0600)
/etc/agent/agent.env          secrets (mode 0600)
/var/lib/agent                SQLite, reports, runtime state
/var/log/agent                logs
/var/backups/agent/snapshots  verified backups
/opt/agent/releases/<SHA>     immutable releases
/opt/agent/current            atomic active-release symlink
```

The deprecated `scripts/btc-agent-24h.sh` is intentionally inert. Termux:Boot,
cron, PM2, V1 service units, and user-level scheduler units must not launch a
second scheduler. For the unprivileged profile, the immutable user service above is
the sole exception; legacy `btc-agent-v2.service` remains disabled. Do not clear
operator halt as part of deployment or restart.

## Pre-release gate

1. Worktree is clean and the exact Git SHA is reviewed.
2. CI passes format, tests, vet, race tests, Linux build, migration and secret scan.
3. Build the binary outside production and record SHA-256.
4. Keep operator halt active.
5. Run `deploy/backup.sh`, then `deploy/verify-backup.sh <archive>`.
6. Confirm exactly one scheduler is enabled for the selected profile and no legacy
   Termux boot loop, cron, PM2 entry, V1 unit, or `btc-agent-v2.service` can run.
7. Retain the preceding immutable release and its verified backup.

## Shadow verification

With the protected service environment loaded, run the commands in
[production-verification-checklist.md](production-verification-checklist.md).
Record UTC start/end, Git SHA, binary SHA, owner instance, fencing token, lease
freshness, operator halt, reconciliation, outbox, dashboard freshness, alerts and
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
loss: request/activate operator halt, preserve DB/WAL/outbox/logs, snapshot remote
balances/positions/orders, and reconcile before retrying. Never send a replacement
order with a new client ID until the original outcome is known.
