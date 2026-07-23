# Production Verification Checklist

Use this checklist after deploying a reviewed immutable release. It does not authorize
live execution. Keep operator halt active unless a separately authorized canary step
explicitly requires otherwise.

## Evidence header

Record without secret values:

- UTC start/end time.
- Git SHA and binary SHA-256.
- Service/unit name and instance ID.
- Operator halt state.
- Run mode and whether execution is enabled.
- Backup location/checksum and rollback release SHA.

## Preflight

1. Build and test outside production, including `deploy/test-backup.sh`, `deploy/test-health-check.sh` and `deploy/test-service-unit.sh`.
2. Run `deploy/backup.sh`, then `deploy/verify-backup.sh <archive>`; verify the backup checksum, manifest and every SQLite `PRAGMA quick_check` pass.
3. Confirm the rollback release and `deploy/rollback.sh` target exist.
4. Run the verifier for the selected production profile on the Linux host:
   - root-managed: `deploy/verify-runtime.sh`;
   - unprivileged immutable user service: `deploy/systemd/verify-immutable-user-service.sh`.
   It must show exactly one immutable scheduler; no V1 cron/service, PM2 entry,
   legacy user service, or Termux boot loop may execute. Do not remove V1 rollback
   files yet.
5. Confirm config and environment files are owner-only. Never print their contents.

Stop on missing backup, ambiguous service ownership, unexpected process, secret
exposure, or an unknown release SHA.

## Halted/reconcile-only verification

With credentials loaded by the protected service environment, run:

```bash
./bin/btc-agent live-doctor --config config.yaml
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent live-auto-audit --config config.yaml
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true \
  ./bin/btc-agent live-supervisor --config config.yaml --dry-run
```

Record:

- doctor verdict and blockers;
- local open/unknown orders;
- remote pending/remote-only orders and identity conflicts;
- reconciled positions and residual reservations;
- execution owner, fencing token and lease expiry/freshness;
- outbox pending/dead-letter/stale claims;
- Supabase/R2 delivery status, or `DISABLED` when credentials are intentionally absent;
- dry-run `desired`, `would_place`, `placed`, `canceled` and exchange call count.

Required result before shadow observation:

- exactly one fresh execution owner;
- the selected runtime verifier passes (`deploy/verify-runtime.sh` or
  `deploy/systemd/verify-immutable-user-service.sh`);
- monotonic fencing token after restart;
- clean reconciliation with no remote-only/unknown/identity conflict;
- no stale data or unavailable protection snapshot;
- dry-run exchange calls equal zero;
- `No real order was placed.`

`APPROVED_MONITORING` and `APPROVED_DRY_RUN` are not real-order approval.

## Shadow/canary observation

Observe for at least seven continuous days with operator halt active. Exercise a
controlled restart and a full reboot, and test delivery of each critical alert. Observe
scheduler restarts, lease renewal, reconcile cycles, order/fill deltas, capital
reservations, alerts, outbox delivery and dashboard freshness for the approved window.
Record every skipped check and reason. Any stale lease/data, unknown order, reconcile
mismatch, duplicate submission, dead-letter growth, unavailable protection snapshot,
failed critical alert, unexplained balance/position delta, or second scheduler fails
the verification.

Only an authorized operator may clear halt or approve canary/real execution. The
application must still enforce `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` and
all final execution assertions.

## Cutover and V1 retention

Merge/cut over only after the reviewed SHA, halted verification, shadow window and
operator approval all pass. Retain V1 backup/service artifacts through the rollback
window. Run `deploy/cleanup-v1.sh --execute` only after health, reconciliation, backup
and `AGENT_CLEANUP_APPROVED` checks pass.

## Final audit statement

The audit must state one of:

- `CODE_READY_ONLY`
- `APPROVED_MONITORING`
- `APPROVED_DRY_RUN`
- `APPROVED_CANARY`
- `APPROVED_REAL_ORDER`
- `BLOCKED`

Include the exact evidence window and end with either `No real order was placed.` or a
reconciled list of explicitly authorized canary order IDs. Never infer current runtime
state from documentation alone.
