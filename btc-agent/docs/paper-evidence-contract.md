# Paper lifecycle evidence contract

## Purpose

Measure whether paper orders pass through a complete, observable lifecycle under
the real scheduler and real market data. This is **not** evidence of real
exchange execution quality, profitability, or permission to trade live.

## Boundaries

- Runtime remains `app.mode=paper`, `paper_trading=true`,
  `real_trading_enabled=false`, and operator halt `ACTIVE`.
- Do not insert synthetic paper orders, alter production SQLite, force a plan,
  clear halt, place/cancel a real order, or tune thresholds to manufacture data.
- Only lifecycle records created naturally by the scheduler are admissible.

## Evidence window

Record a UTC start/end and immutable release SHA. Preserve daily scorecards,
observer rows, runtime verification output, and any data-health/reconciliation
blockers for the whole window.

The minimum target is **30 total orders and 15 terminal outcomes across at
least 7 calendar days**. This is a review threshold, not an automatic readiness
verdict. If market conditions do not naturally create the sample, the result is
`INSUFFICIENT_EVIDENCE` and the window extends.

## Required lifecycle observations

For every order, retain symbol, created time, intended price/notional, status,
terminal time when present, and terminal reason. The aggregate review must show:

- total/open/terminal count;
- filled, invalidated, expired, and cancelled counts;
- fill and invalidation rate over terminal orders;
- average and maximum open age;
- counts per symbol;
- unknown status count, expected to be zero;
- market/data-health and scheduler outages that overlap the window.

## Review gates

The reviewer must reject evidence when any condition is true:

- missing continuity evidence or unverifiable release/runtime;
- unknown paper status;
- stale or missing source data without a documented fail-closed result;
- duplicate lifecycle records or non-idempotent transitions;
- unbounded open-order age without a documented expiry/cancel policy;
- operator halt not active, or any real order/cancel activity;
- sample created manually or through production DB mutation.

A passing paper review only permits a separate human discussion about further
validation. It does not enable sizing changes, canary, or live execution.

## Daily evidence commands

```bash
/home/admin/btc-agent/immutable/current/agent paper-scorecard   --config /home/admin/btc-agent/runtime/config.yaml
/home/admin/btc-agent/immutable/verify-runtime.sh
```

Archive outputs with the UTC date and release SHA. Never treat a scorecard alone
as runtime truth; pair it with immutable verifier and observer evidence.
