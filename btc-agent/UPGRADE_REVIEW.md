# Current Live-Auto Scenario

This file replaces old upgrade notes. Historical plan content was removed because current runtime is now the live-auto supervisor scenario.

## Runtime authority

Production runtime is one path:

```text
scripts/btc-agent-scheduler.sh
-> btc-agent scheduler --run-now
-> live-supervisor
-> managed order engine
```

Normal live desired orders require all of:

```text
BTC_AGENT_MODE=live-auto
BTC_AGENT_ALLOW_AUTO_LIVE=true
live.enabled=true
live.auto_execute=true
live.require_manual_confirm=false
live.proof_only=false
live.order_management_enabled=true
live.supervisor_enabled=true
execution.real_trading_enabled=true
plan.State=ACTIVE_LIMIT
plan.ActionPermission=ALLOWED
BTC accumulation phase=ACCUMULATION_CONFIRMED
asset.State=ACTIVE_LIMIT
```

## Removed old scenario

Old canary/auto-ladder production logic is no longer part of the current scenario.

- No legacy canary auto runtime mode.
- No production auto-ladder branch.
- No canary config fallback.
- Managed order engine is the live-auto execution path.

## Live operations monitoring

Selected upgrade content from `Agent-main-advanced-operations-upgrade.zip` was ported only for live operations:

- `operations-plan` builds read-only capital/exposure/trigger plan.
- `market-watch` refreshes data, analysis, plan, reports, and Telegram state alerts.
- Scheduler can run market-watch between supervisor cycles with backoff/error alerts.
- Canary/demo/legacy auto-ladder code remains excluded.

These features do not place, cancel, or override orders. Execution remains supervisor + managed order engine only.

## Architecture load reduction

Current upgrade also split overloaded command/scheduler code into focused files while keeping one `package main` and the same runtime behavior:

- `cmd_market.go`, `cmd_ops.go`, `cmd_live.go`, `cmd_supervisor.go`, `cmd_reconcile.go`, `cmd_research.go`, `cmd_notify.go`, `cmd_status.go`, `cmd_maintenance.go`, `cli.go` split command responsibilities.
- `scheduler_heartbeat.go`, `scheduler_lock.go`, `scheduler_backoff.go`, `scheduler_telegram.go`, `scheduler_time.go` split scheduler helpers while keeping `runScheduler` as the only orchestration loop.
- SQLite `runtime_events` stores read-only ops signals for market-watch and live-supervisor events.
- `ops-events` reads pending ops events. It does not place, cancel, or override orders.

Order authority remains single-writer: live-supervisor + managed order engine only.

## Microstructure observation

Milestone B adds report-only microstructure plumbing:

- `microstructure-fetch` reads Binance public spot/futures observation only: taker flow/CVD, orderbook imbalance/spread, open interest, funding, spot-perp basis.
- SQLite `microstructure_snapshots` stores latest observation evidence.
- `market-watch` can fetch microstructure and write runtime events.
- `operations-plan` and `real-data-survey` show microstructure status and blockers.
- Futures data is observation-only. No futures execution, no leverage, no market order.
- If `microstructure.require_fresh_for_active=true`, stale/missing microstructure can only reduce authority: BTC max `WATCH`, asset cannot stay `ACTIVE_LIMIT`.

## Pre-live safety hardening

Before autonomous real-order approval, live-auto now has extra production checks:

- `live-auto-audit` writes `reports/live_auto_audit_latest.md/json` and returns `APPROVED_MONITORING`, `APPROVED_DRY_RUN`, `APPROVED_REAL_ORDER`, or `BLOCKED`.
- Managed order engine runs a final execution assertion immediately before `PlaceSpotLimitOrder`.
- Final assertion blocks non-`ACTIVE_LIMIT`, non-`ALLOWED`, non-`BUY limit post-only`, unsafe config, wrong risk flags, invalid size, or cap overflow.
- Forced `ACTIVE_LIMIT` simulation proves dry-run `would_place` behavior without exchange submission.
- First-order quarantine can restrict first real live order to one small layer after dry-run audit.
- `market-watch` emits near-unlock runtime events when BTC/plan approaches real-order readiness.

## Safety invariants

- `WATCH`, `SCOUT`, and `ARMED` are not order authority.
- Live allocator may rank/size only already-qualified `ACTIVE_LIMIT` assets.
- `OpportunityComposite` is used for allocation score only inside existing guards.
- Spot limit BUY post-only only.
- No futures, no leverage, no market order.
- Telegram commands are read-only.
- Research reports and `real-data-survey` do not edit config or place orders.
- BTC accumulation detector is an extra deterministic gate; it does not bypass `ACTIVE_LIMIT + ALLOWED`.

## Operator status target

Current desired operating state:

```text
scheduler=running
mode=live-auto
dry_run=false
bot_ready=true
market_ready=false until ACTIVE_LIMIT+ALLOWED
can_submit=false until ACTIVE_LIMIT+ALLOWED
```

When market is not ready, expected managed cycle remains:

```text
desired=0
placed=0
canceled=0 unless stale open order needs cleanup
blocked may explain gates
```
