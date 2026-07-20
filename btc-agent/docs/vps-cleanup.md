# V1 Cleanup

`deploy/cleanup-v1.sh` is dry-run by default. Execution requires healthy V2, stopped
V1, explicit approval, reconciled orders and retained backups. It never removes the
V2 config, secrets, data directory, backups or active release.
