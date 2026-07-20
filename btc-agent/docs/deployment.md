# VPS Deployment

Build/test outside production, install immutable binaries under
`/opt/agent/releases/<git-sha>/agent`, atomically repoint `/opt/agent/current`, keep
config/secrets under `/etc/agent`, local state under `/var/lib/agent`, logs under
`/var/log/agent`, and backups under `/var/backups/agent`. Never run from a Git clone.
Live deployment requires manual approval, backup, reconcile-only startup and health.
