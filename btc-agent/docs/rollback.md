# Rollback

Disable execution and halt first, reconcile OKX, preserve SQLite, then use
`deploy/rollback.sh` to repoint the previous immutable release. Re-run health and
reconciliation before considering execution. Never delete the last known-good release.
