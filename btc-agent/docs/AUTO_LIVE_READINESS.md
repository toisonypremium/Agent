# Auto-Live Readiness Runbook

## Safety boundary

This repository is configured to fail closed. Auto-live can submit an order only
when every gate below is satisfied. This runbook does not authorize real trading.

The dashboard and Telegram are read-only. They cannot place, cancel, resume, or
modify orders.

## Required configuration

Keep `config.yaml` local and untracked. Before considering auto-live, ensure the
local configuration has all of the following values:

```yaml
app:
  mode: "live"

live:
  enabled: true
  auto_execute: true
  require_manual_confirm: false
  proof_only: false
  live_auto_mode: true
  order_management_enabled: true
  supervisor_enabled: true
  management_interval_minutes: 15

execution:
  paper_trading: false
  real_trading_enabled: true
```

The existing validation rejects incomplete auto-live configurations. The runtime
also requires the separate process-scoped opt-in:

```bash
export BTC_AGENT_ALLOW_AUTO_LIVE=true
```

Do not add this variable to a shell profile. Keep it in the protected runtime
service environment only after operator approval.

## Preflight sequence

Run each command from the repository root. Do not treat a warning, missing data,
or a missing audit as approval.

```bash
make verify
./bin/btc-agent live-doctor --config config.yaml
./bin/btc-agent live-readiness --config config.yaml
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true \
  ./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent live-auto-audit --config config.yaml
```

The preflight must show all of the following before a separate operator approval
is considered:

- `DOCTOR_OK`, fresh data, clean reconciliation, and no risk-governor block.
- Plan state `ACTIVE_LIMIT` and permission `ALLOWED`.
- BTC accumulation phase `ACCUMULATION_CONFIRMED`.
- A current-plan dry-run with a proposed order and no final assertion block.
- A fresh audit whose verdict is `APPROVED_REAL_ORDER`.
- Valid OKX credentials and account/preflight checks.
- Operator halt is inactive.

A passing configuration alone never grants authority. The market and evidence
checks above are evaluated again on every supervisor cycle.

## Start criteria

Only an explicitly authorized operator may start the existing scheduler wrapper
in `live-auto` mode. The wrapper requires the explicit environment opt-in and
runs the live doctor before starting the scheduler:

```bash
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true \
  scripts/btc-agent-scheduler.sh
```

Do not start this command until all preflight evidence is fresh and the operator
has separately approved real execution.

## Monitoring and emergency stop

Use the read-only dashboard or these commands:

```bash
./bin/btc-agent status --config config.yaml
./bin/btc-agent operator-status --config config.yaml
./bin/btc-agent live-doctor --config config.yaml
```

To halt through the controlled CLI path:

```bash
./bin/btc-agent operator-halt --config config.yaml
```

Never automatically resume after a halt. Re-run the entire preflight sequence,
review the reason for the halt, and obtain a new operator authorization.

## Rollback

1. Run `operator-halt`.
2. Stop the scheduler process or service.
3. Preserve the database and reports for review; do not delete or rewrite them.
4. Review open orders with `reconcile-live-orders --dry-run`.
5. Do not cancel an actual order unless the operator explicitly directs it.

## Non-negotiable invariants

- Spot limit BUY, post-only only.
- No futures, leverage, shorts, market orders, or automatic sells.
- `WATCH`, `SCOUT`, and `ARMED` cannot create normal live orders.
- Missing, stale, or inconsistent evidence blocks execution.
