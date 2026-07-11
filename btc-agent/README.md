# btc-agent

Rule-based BTC market gate + ETH/SOL/RENDER accumulation planner for Termux/root Android.

## Safety invariants

- Default config is safe: paper/simulation only, live disabled, proof-only enabled, real trading disabled.
- Production runtime, when explicitly enabled, is one path: `scheduler` in `live-auto` mode -> `live-supervisor` -> managed order engine.
- Normal live desired orders require deterministic `ACTIVE_LIMIT` plan, BTC permission `ALLOWED`, and BTC `ACCUMULATION_CONFIRMED` gate evidence.
- `WATCH`, `SCOUT`, and `ARMED` are observation/explanation states. They do not create normal live orders.
- Live order type is spot limit BUY post-only only.
- No futures, no leverage, no market order.
- Telegram commands are read-only. No Telegram buy/sell/cancel/override path.
- Research/report layers can explain or rank opportunities, but cannot override Agent 1/2 or live safety gates.

## Termux install

```bash
pkg update
pkg install golang git ca-certificates
cd /data/data/com.termux/files/home/.openclaw/workspace/btc-agent
go mod tidy
go build -o bin/btc-agent .
cp config.yaml.example config.yaml
```

Do not commit real `config.yaml`, `.env`, DB, logs, reports, or binaries.

## Core commands

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent analyze --config config.yaml
./bin/btc-agent plan --config config.yaml
./bin/btc-agent status --config config.yaml
./bin/btc-agent run-daily --config config.yaml
./bin/btc-agent paper-manager --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent backtest-live-manager --config config.yaml
./bin/btc-agent real-data-survey --config config.yaml
./bin/btc-agent learn --config config.yaml
./bin/btc-agent universe-research --config config.yaml
./bin/btc-agent live-proof --config config.yaml
./bin/btc-agent live-readiness --config config.yaml
./bin/btc-agent live-doctor --config config.yaml
./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent live-positions --config config.yaml
./bin/btc-agent telegram-commands --config config.yaml
./bin/btc-agent scheduler --config config.yaml --run-now --dry-run
```

Manual proof execution still exists, but is separate and requires the exact confirm phrase plus live gates:

```bash
./bin/btc-agent execute-live-proof-order --config config.yaml --confirm I_UNDERSTAND_THIS_PLACES_A_REAL_SPOT_LIMIT_ORDER
```

## Decision pipeline

Agent 1 is BTC market gate/benchmark. BTC is not an accumulation asset. BTC first classifies deterministic market-maker accumulation phase from closed OHLCV data: `MARKDOWN`, `LIQUIDITY_SWEEP`, `SELL_ABSORPTION`, `RECLAIM`, `ACCUMULATION_CONFIRMED`, `DISTRIBUTION`, or `INVALIDATED`.

Agent 2 evaluates configured accumulation assets only. Production config keeps exactly three assets in `data.symbols.assets`. Asset setup can only reach full order authority after BTC is `ACCUMULATION_CONFIRMED`, asset MM/reclaim gates pass, and discount/reward-risk/liquidity/rotation gates pass.

Plan states:

- `NO_TRADE`: hard block or unusable setup.
- `WATCH`: visible candidate, not actionable.
- `SCOUT`: near setup with only soft waits, not order authority.
- `ARMED`: strong watch/probe context, not order authority.
- `ACTIVE_LIMIT`: strict final gates passed and valid layers exist.

Normal managed live desired orders are built only when:

```text
plan.State == ACTIVE_LIMIT
plan.ActionPermission == ALLOWED
BTC accumulation phase == ACCUMULATION_CONFIRMED
asset.State == ACTIVE_LIMIT
```

Then preflight, risk governor, data health, reconcile safety, MM/liquidity gate, notional caps, open-order caps, and post-only checks still apply.

## Live-auto production runtime

Use scheduler/supervisor only:

```bash
export BTC_AGENT_MODE=live-auto
export BTC_AGENT_ALLOW_AUTO_LIVE=true
./bin/btc-agent live-doctor --config config.yaml
./scripts/btc-agent-scheduler.sh
```

Wrapper behavior:

- `paper` and `live-proof` modes run scheduler with `--dry-run`.
- `live-auto` requires `BTC_AGENT_ALLOW_AUTO_LIVE=true`.
- `live-auto` runs `live-doctor` before starting real scheduler.
- Scheduler uses `live-supervisor` at `live.management_interval_minutes`.
- Supervisor calls managed order engine; it does not use old ladder/canary paths.

Live-auto config gates must be explicit:

```text
live.enabled=true
live.auto_execute=true
live.require_manual_confirm=false
live.proof_only=false
live.order_management_enabled=true
live.supervisor_enabled=true
execution.real_trading_enabled=true
BTC_AGENT_ALLOW_AUTO_LIVE=true
```

If any gate fails, bot blocks or reconciles only.

## Allocation logic

Live capital allocator uses `OpportunityComposite` plus history quality only inside existing live guards:

- Non-`ACTIVE_LIMIT` asset: score `0`, `MaxLayers=0`.
- BTC permission not `ALLOWED`: zero live risk budget.
- Data/risk/hard blocker composite verdict: score `0`.
- Quality grade adjusts size only; D blocks, A/B full, C reduced, missing/NO_SAMPLE small probe size.
- Portfolio caps, per-order cap, per-asset cap, total cap, positions, and open orders still cap budget.

This ranks/sizes already-qualified opportunities. It does not open permission for `WATCH`, `SCOUT`, or `ARMED`.

## Reports

Main report files:

```text
reports/latest.md/json
reports/bot_state_latest.md/json
reports/scenario_latest.md/json
reports/filter_attribution_latest.md/json
reports/technical_scorecard_latest.md/json
reports/capital_plan_research_latest.md/json
reports/coin_universe_research_latest.md/json
reports/decision_dashboard_latest.md/json
reports/live_supervisor_latest.md/json
reports/auto_live_management_latest.md/json
reports/live_doctor_latest.md/json
reports/live_readiness_latest.md/json
reports/live_reconcile_latest.md/json
reports/live_position_latest.md/json
```

Research-only reports do not edit config, replace production assets, or place orders.

## Real-data survey and learning

Recommended report-only flow:

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent backtest-live-manager --config config.yaml
./bin/btc-agent real-data-survey --config config.yaml
./bin/btc-agent learn --config config.yaml
```

`real-data-survey` consolidates local candle backtests, BTC accumulation phase/false-positive audit, Agent 1/2 audits, managed live-manager history simulation, and learning actions into `reports/real_data_survey_latest.md/json`. It is diagnostic only: no config write, no OKX live order, no gate override. `learn` includes survey evidence but still requires manual review before any rule/config/code change.

## Telegram read-only management

Allowed commands:

```text
/status
/why
/coins
/filters
/scorecard
/allocation
/capital
/universe
/dashboard
/trigger
/orders
/positions
/doctor
/supervisor
/next
/risk
/help
```

Blocked/not implemented by design:

```text
/buy /sell /market /leverage /override /resume /halt /cancel /close
```

Telegram can show state, blockers, scorecard, allocation research, universe research, dashboard, trigger, orders, positions, doctor, and supervisor summaries. It cannot place/cancel/close orders or override gates.

## Verification

Before reporting code work done:

```bash
gofmt -w .
go test -v -count=1 ./...
go vet ./...
go build -o bin/btc-agent .
```

Dry-run live check:

```bash
set -a; . "$HOME/btc-agent.env"; set +a
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true ./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent live-doctor --config config.yaml
```

Expected while market is not `ACTIVE_LIMIT + ALLOWED`:

```text
desired=0
placed=0
can_submit=false
No real order was placed.
```

## Secrets

- Never paste OKX keys, Telegram token, `.env`, or real config secrets into chat/logs.
- Use `$HOME/btc-agent.env` for runtime env.
- Use OKX IP whitelist and least permissions.
- Rotate any key pasted outside the device.
