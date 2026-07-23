# Immutable VPS deployment

The supported production profile is an unprivileged systemd user service. It runs
one immutable scheduler from `%h/btc-agent/immutable/current/agent` and preserves
all mutable state in `%h/btc-agent/runtime`.

> [!IMPORTANT]
> Deploying a binary never clears the operator halt and never authorizes real
> execution. Real canary approval remains a separate operator action.

## Layout

```text
%h/btc-agent/immutable/current/agent   active immutable binary
%h/btc-agent/runtime/config.yaml       protected runtime config
%h/btc-agent/runtime/data/             SQLite state
%h/btc-agent/runtime/backups/          WAL-safe verified snapshots
%h/btc-agent/runtime/soak/             halted-shadow observations
```

## Build and preflight

Run outside the VPS runtime directory:

```bash
go test -count=1 ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/agent .
bash deploy/test-backup.sh
bash deploy/test-health-check.sh
bash deploy/test-service-unit.sh
bash deploy/test-immutable-runtime.sh
```

Record the binary SHA-256 and create a new release directory. Never overwrite an
existing release. Atomically update only the `current` symlink after checksum
verification.

## Install systemd support

From a checked, approved source release on the VPS:

```bash
bash deploy/systemd/install-immutable-user-service.sh
bash deploy/systemd/verify-immutable-user-service.sh
```

The installer enables:

- `btc-agent-immutable.service`
- `btc-agent-immutable-observe.timer` — hourly read-only evidence
- `btc-agent-immutable-backup.timer` — daily SQLite-safe snapshot

## Post-deploy gates

```bash
systemctl --user is-active btc-agent-immutable.service
/home/admin/btc-agent/immutable/current/agent operator-status \
  --config /home/admin/btc-agent/runtime/config.yaml
bash deploy/systemd/verify-immutable-user-service.sh
```

Expected results:

- Exactly one scheduler process.
- Lease owner `immutable-shadow-01`, fresh expiry and positive fencing token.
- Fresh heartbeat and SQLite `PRAGMA quick_check=ok`.
- Operator halt remains `ACTIVE` unless separately and explicitly authorized.

## Backup and restore drill

The daily timer runs `immutable/backup.sh`. It creates a SQLite online-backup
snapshot plus checksums. Verify a snapshot before relying on it:

```bash
/home/admin/btc-agent/immutable/verify-backup.sh \
  /home/admin/btc-agent/runtime/backups/snapshot-<timestamp>.tar.gz
```

Do not replace the live database during a drill. Restore only into an isolated
directory and validate `PRAGMA quick_check` before any manual recovery decision.

## Rollback

Rollback means atomically repointing `immutable/current` to a previously verified
release, then restarting the user service. Preserve runtime SQLite files,
backups, lease evidence, and operator halt. Reconcile before considering any
execution change.
