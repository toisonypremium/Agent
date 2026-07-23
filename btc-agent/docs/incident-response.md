# Incident Response

## First five minutes

1. Activate global/operator halt. Do not clear it automatically.
2. Stop new execution only; retain the scheduler for reconcile-only operation when safe.
3. Preserve the SQLite database including WAL/SHM, reports and logs.
4. Snapshot OKX balances, positions, open orders, fills and client order IDs.
5. Record UTC timestamps, release SHA, binary SHA, execution owner, fencing token,
   lease expiry, heartbeat age and the triggering evidence.
6. Notify the operator through the tested critical-alert path. If the primary channel
   fails, use the documented secondary path.

## Scenario playbooks

| Trigger | Required action before recovery |
|---|---|
| Unknown/duplicate order or partial fill | Halt, reconcile by client/order ID, reserve capital conservatively; never submit a replacement until resolved. |
| OKX timeout or network split | Treat the submit result as unknown; reconcile remote state before any retry. |
| Stale heartbeat, lease or data | Halt execution, restore fresh ownership/data, then reconcile. |
| SQLite corruption or disk full | Stop writes, preserve files, restore only a verified backup, run `PRAGMA quick_check`, then reconcile. |
| Clock drift | Halt execution, correct time synchronization, verify timestamp-sensitive exchange and lease flows, then reconcile. |
| Credential suspicion | Halt, preserve evidence, rotate credentials outside the application, validate least privilege and reconcile before restart. |
| Alert delivery failure | Treat critical-alert delivery as failed monitoring; repair and send a synthetic critical alert before recovery. |

## Recovery

Root cause, fresh ownership, clean reconciliation, verified backup/restore where
applicable, healthy heartbeat and successful alert delivery are all required before
an authorized operator considers changing halt state. Recovery evidence must name
the exact immutable release. Never delete the last known-good release.
