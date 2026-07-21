# V1 Removal Report

Status: **not removed**. V1 remains required for shadow comparison and rollback.
Code readiness alone cannot change this status. Cleanup requires the reviewed release,
current halted/reconcile-only verification, approved shadow window, operator approval
and a retained backup/rollback release. After approved cutover, update this report with
removed services/cron/binaries, retained backups and locations, rollback procedure,
remaining compatibility code and the date when the final backup may be deleted.
`deploy/cleanup-v1.sh` remains dry-run by default and requires
`AGENT_CLEANUP_APPROVED` for execution.
