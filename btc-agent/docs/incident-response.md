# Incident Response

For stale data, ownership loss, duplicate/unknown order, reconcile mismatch or
credential suspicion: activate global/operator halt, disable execution, preserve
logs/SQLite/outbox, snapshot OKX balances/positions/orders, notify the operator, and
classify impact. Rotate exposed credentials outside the application after evidence
capture. Restore only after root cause, reconciliation and ownership validation.
