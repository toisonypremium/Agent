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
asset.State=ACTIVE_LIMIT
```

## Removed old scenario

Old canary/auto-ladder production logic is no longer part of the current scenario.

- No legacy canary auto runtime mode.
- No production auto-ladder branch.
- No canary config fallback.
- Managed order engine is the live-auto execution path.

## Safety invariants

- `WATCH`, `SCOUT`, and `ARMED` are not order authority.
- Live allocator may rank/size only already-qualified `ACTIVE_LIMIT` assets.
- `OpportunityComposite` is used for allocation score only inside existing guards.
- Spot limit BUY post-only only.
- No futures, no leverage, no market order.
- Telegram commands are read-only.
- Research reports and `real-data-survey` do not edit config or place orders.

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
