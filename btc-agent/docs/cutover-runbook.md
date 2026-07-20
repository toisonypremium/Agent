# Cutover Runbook

1. Backup V1; record OKX balances, positions and open orders.
2. Set global/operator halt and stop V1 scheduler after current work finishes.
3. Prove no V1 process/cron can execute.
4. Start V2 with `RUN_MODE=shadow` and execution disabled; compare outputs.
5. Start reconcile-only; resolve every mismatch and unknown order.
6. Acquire the V2 lease/fencing token; keep execution disabled.
7. After manual approval, enable one canary path and monitor heartbeat/order lifecycle.
8. Roll back on stale data, lost ownership, reconcile mismatch or health failure.
