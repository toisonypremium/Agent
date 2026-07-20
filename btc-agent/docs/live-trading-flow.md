# Live Trading Flow

Before any OKX submission the bot must prove: live mode and execution enabled;
active non-expired ownership lease and fencing token; no global/operator halt; fresh
market and balance data; successful reconciliation; unique idempotency/client order
ID; valid plan/allocation/risk; valid instrument precision, size and notional.

```text
Plan -> reconcile OKX -> ownership/fence -> halt/freshness/risk/preflight
-> reserve local intent -> submit OKX -> persist response -> reconcile lifecycle
```

An ambiguous network result remains pending reconciliation. It must never trigger a
blind retry with a new client order ID.
