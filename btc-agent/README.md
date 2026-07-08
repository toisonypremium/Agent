# btc-agent

Rule-based BTC market analyst + ETH/SOL/RENDER accumulation planner for Termux/root Android.

## Safety

- Default is paper trading/simulation.
- No futures, no leverage.
- Live execution code exists, but it is disabled/fail-closed by default.
- Real spot-limit orders require explicit config, manual confirmation or gated auto runtime, proof mode off, canary/operator gates, and deterministic `ACTIVE_LIMIT`.
- If edge is unclear, output is `NO_TRADE` or `WATCH`.

## Termux install

```bash
pkg update
pkg install golang git ca-certificates
cd /data/data/com.termux/files/home/.openclaw/workspace/btc-agent
go mod tidy
go build -o bin/btc-agent .
cp config.yaml.example config.yaml
```

## Commands

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent analyze --config config.yaml
./bin/btc-agent plan --config config.yaml
./bin/btc-agent paper-manager --config config.yaml
./bin/btc-agent run-daily --config config.yaml
./bin/btc-agent run-ai-watch --config config.yaml
./bin/btc-agent status --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent learn --config config.yaml
./bin/btc-agent export-training --config config.yaml
./bin/btc-agent eval-ai --config config.yaml
./bin/btc-agent live-proof --config config.yaml
./bin/btc-agent research-doctor --config config.yaml
./bin/btc-agent research-brief --config config.yaml
./bin/btc-agent execute-live-proof-order --config config.yaml --confirm <client-order-id>
./bin/btc-agent auto-live-order --config config.yaml
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent live-positions --config config.yaml
./bin/btc-agent maintenance --config config.yaml
```

## Telegram/ntfy

Edit `config.yaml` notify section. Leave disabled unless configured.

## Repo hygiene

`config.yaml` is local-only and ignored. Commit `config.yaml.example`, not real local config or secrets.

Use the standard verification gate before reporting work done:

```bash
make verify
```

Future coding agents should follow `AGENT_DONE_CHECK.md`: report changed files, exact command results, and remaining real risks. If no source/test/docs files changed, they must say `NOT DONE - no source files changed`.

## Decision pipeline

Agent 1 is BTC market gate/benchmark. BTC is not an accumulation target. Agent 2 evaluates only configured ETH/SOL/RENDER assets.

Decision reasons are typed internally:

- `HARD_BLOCK`: safety block. No order. Examples: panic selling, confirmed falling knife, confirmed distribution/bull-trap, invalid data.
- `SOFT_WAIT`: setup is not ready yet, but can stay visible as `SCOUT`/watch candidate. Examples: BTC not `ALLOWED`, downtrend without panic, neutral flow, rotation/rank wait, RR gap, discount gap.
- `INFO`: context only.

Plan states:

- `NO_TRADE`: hard blocker or unusable setup.
- `WATCH`: candidate visible but weak or missing context.
- `SCOUT`: near setup with only soft waits. Never creates orders.
- `ARMED`: BTC/asset candidate strong enough to watch/probe context. Never creates orders through `OrdersFromPlan`.
- `ACTIVE_LIMIT`: strict final gates passed and valid layers exist. Only this state can create paper/live-proof orders.

`OrdersFromPlan` emits orders only for `ACTIVE_LIMIT`. `SCOUT` and `ARMED` are diagnostic/planning states, not execution authority.

## Paper order manager

`paper-manager` advances paper-only orders using stored candles and the latest deterministic plan. It can mark paper orders `FILLED`, `EXPIRED`, `CANCELLED`, or `INVALIDATED`, writes `reports/paper_manager_latest.md/json`, and never calls live exchange order or cancel endpoints.

## Disabled live order manager

Managed live execution remains disabled by default. When explicitly enabled with all live gates, it uses DB lifecycle states `PLANNED`, `SUBMITTED`, `PARTIAL_FILL`, `FILLED`, `CANCELLED`, `EXPIRED`, and `REJECTED`; new managed submissions are pre-reserved in SQLite before exchange submit. Normal verification and tests use fake exchange paths and do not place real orders.

## Liquidity Flow Engine

`internal/flow` adds deterministic OHLCV-only MM/liquidity-flow detection:

- sweep low/high
- support reclaim
- failed breakdown / failed breakout
- absorption near support
- distribution near resistance
- multi-timeframe flow bias and confidence

This is not AI price prediction. Good flow can only help Agent 1 move toward `ARMED` when other risk gates agree. Bad flow can block FOMO/trap entries. It does not bypass `NO_TRADE`, falling-knife, FOMO, no-futures, no-leverage, or paper-only rules.

## Reports and status

Daily report files are written to:

```text
reports/latest.md
reports/latest.json
```

Report starts with `BTC DAILY MARKET BRIEF` and includes: quick conclusion, multi-timeframe analysis, zones, risks, MM/liquidity flow, Agent 2 plan, scenarios, action conclusion.

`status` prints the latest regime, permission, risk, zones, liquidity flow, Agent 2 state, per-asset plan, and open paper order count.

`maintenance` prunes old report rows, old live event rows, closed paper-order history over the configured cap, and old unprotected files in `reports/`. Scheduler can run the same cleanup automatically when `maintenance.enabled` and `maintenance.scheduler_enabled` are true. It does not prune candles, live orders, fills, positions, or operator halt settings.

## Backtest + Flow Audit

Run after `fetch` has stored BTC 1D candles in SQLite:

```bash
./bin/btc-agent backtest --config config.yaml
```

Backtest reads local candles only. It does not call exchange APIs, does not read secrets, and does not place orders. It writes:

```text
reports/backtest_latest.md
reports/backtest_latest.json
```

Use this to audit Liquidity Flow signals by historical forward returns and drawdowns. It also runs BTC Flow Detector Bottleneck Audit, which reports flow component frequency, bias forward return/drawdown, and audit-only parameter sensitivity; it does not tune `flow.DefaultParams()` automatically. Flow Param Candidate Forward Quality Audit then compares those audit-only param candidates by bullish/bearish forward return, added signal quality, false-positive proxy, and drawdown; it also does not tune production params automatically. It also runs BTC Permission Bottleneck Audit, which reports Agent 1 permission distribution, forward return/drawdown by permission, and top blockers explaining why BTC is not `ALLOWED`; this is diagnostic only and does not tune Agent 1 automatically. It also runs Agent 2 Layer Simulation for ETH/SOL/RENDER using local 1D candles: limit-layer placement, fills, expiries, invalidation hits, max deployed capital, drawdown, and simulated PnL. Agent 2 diagnostics explain Agent 1 permission counts, regime/risk gates, per-asset block reasons, and sample PLAN/FILL/EXPIRE/INVALIDATION/TAKE_PROFIT/TIME_STOP events. Orders become active from the next candle to avoid same-candle lookahead. The Layer Audit compares invalidation buffers and layer-depth multipliers to see whether volatile assets like RENDER fail because stops are too tight or layers are too shallow. The Exit / Take-Profit Audit compares TP percentages and time-stop windows using local candles only; if TP and invalidation happen in the same candle, invalidation wins as the conservative OHLCV assumption. Agent 2 also uses an Asset Relative Strength Filter when BTC 1D benchmark candles are available: assets that are both weak in absolute momentum and underperforming BTC over the configured lookback are blocked before layer planning. Agent 2 then reports Asset Ranking / Rotation Score, combining relative strength, momentum, discount quality, and asset-level liquidity flow; low-score or low-rank assets stay WATCH before layer planning. Agent 2 also requires Asset Flow Entry confirmation before layer planning: sweep-low reclaim, failed breakdown/bear trap, absorption, or accumulation near support can pass; distribution, bull-trap, or failed breakout hard-blocks entry. Candidate Discovery / Watchlist Report appears in `plan`, `status`, and `reports/latest.json`; it ranks the closest candidates by readiness, lists missing conditions, gives the next trigger to wait for, and includes a Strict Entry Checklist for BTC permission, relative strength, rotation score/rank, asset flow entry, discount zone, reward/risk, falling knife, and FOMO. Backtest also includes Watchlist Trigger Audit, which measures forward return, win rate, and worst drawdown after historical actionable watchlist candidates appear, plus Checklist Pass-Count Audit, which reports average checks passed, hard/soft fail rates, near-actionable samples, and top blockers per asset. Watchlist readiness is noise-capped: BTC-not-allowed/downtrend, relative-weak, falling-knife, FOMO, and unconfirmed-flow candidates remain visible for context as WATCH/SCOUT/ARMED tiers but are not actionable by default. Flow neutral is a soft wait by default; only confirmed bearish flow hard-blocks. Ranking, flow entry, watchlist reporting, Strict Entry Checklist, Checklist Pass-Count Audit, BTC Flow Detector Bottleneck Audit, Flow Param Candidate Forward Quality Audit, BTC Permission Bottleneck Audit, and trigger audit do not reallocate capital, trigger alerts, tune rules, or enable real trading. Backtest diagnostics count these blocks in per-asset reasons. These audits do not change production planner/config automatically and do not enable take-profit order placement. They are for debugging rules, not a profit guarantee. If the sample is small, treat the conclusion as weak.

Phase 6 backtest realism adds research-only simulation helpers. `internal/exchange/simulator` provides a fake OKX client for tests with instrument filters, balance reservation/release, submit/cancel, partial fills, full fills, and maker-fee accounting. Liveguard tests run the live order manager against Fake OKX only; they do not call OKX. Backtest output includes a Walk-Forward Split Audit that trains/evaluates Agent 2 simulation across local candle windows without tuning config. Same-candle OHLCV ambiguity remains conservative: if take-profit and invalidation are both crossed in one candle, invalidation wins. These realism helpers do not change production sizing, live gates, BTC permission authority, or `ACTIVE_LIMIT` order authority.

## Learning Recommendations Report

Run after local BTC/assets 1D candles exist in SQLite:

```bash
./bin/btc-agent learn --config config.yaml
```

This reruns deterministic backtest/audits from local candles and writes:

```text
reports/learning_latest.md
reports/learning_latest.json
```

It turns diagnostics into manual recommendations for flow params, BTC blockers, watchlist triggers, layering, and exits. It never edits config, calls an LLM, calls an exchange, places orders, or overrides the deterministic engine.

## AI Training Dataset Export

Run after local BTC/assets 1D candles exist in SQLite:

```bash
./bin/btc-agent export-training --config config.yaml
```

This exports deterministic historical decision snapshots to:

```text
data/training/decision_dataset.jsonl
data/training/decision_dataset.csv
```

The export uses local candles only. It does not call LLMs, does not call exchange APIs, does not read secrets, and does not trade. Labels are heuristic/outcome labels for future AI evaluation or training preparation, not profit guarantees. AI should learn to explain and audit deterministic decisions, not override them.

## Read-only research layer

P3 adds RSS-first research reports inspired by Agent-Reach routing/doctor patterns, but it stays read-only:

```bash
./bin/btc-agent research-doctor --config config.yaml
./bin/btc-agent research-brief --config config.yaml
```

It writes:

```text
reports/research_doctor_latest.md
reports/research_doctor_latest.json
reports/research_brief_latest.md
reports/research_brief_latest.json
```

Scope: public RSS/news only in P3. No exchange auth, no browser cookies, no social login, no order authority. Research can add evidence/context and Telegram summaries, but it cannot place orders, cancel orders, override Agent 1/2, or loosen live safety gates. If `research.enabled=true`, scheduler runs a brief at `research.brief_interval_minutes` and logs warnings without stopping live supervisor.

## AI Agent Evaluation Harness

Run after `export-training` has created the local decision dataset:

```bash
./bin/btc-agent eval-ai --config config.yaml
```

This generates offline evaluation prompts at:

```text
data/ai_eval/eval_cases.jsonl
```

Send those prompts to an AI agent manually or with a future adapter, then save structured JSONL responses to:

```text
data/ai_eval/responses.jsonl
```

Re-run:

```bash
./bin/btc-agent eval-ai --config config.yaml
```

Reports are written to:

```text
reports/ai_eval_latest.md
reports/ai_eval_latest.json
```

The evaluator scores decision accuracy, blocker recall, and safety discipline. It is offline/local-file only: no LLM calls, no exchange API calls, no secrets, and no trading. AI responses are evaluated only; the deterministic engine remains authority.

## AI Watch + Live Readiness Proof

`run-ai-watch` runs the deterministic daily flow, optionally asks the configured 9Router/OpenAI-compatible model to explain the deterministic decision, safety-filters the AI text, and sends Telegram if notify is enabled:

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:20128/v1"
export ANTHROPIC_API_KEY="<local key>"
export MODEL="Claw"
./bin/btc-agent run-ai-watch --config config.yaml
```

Do not store API keys in the repo. AI is reporter/auditor only; it cannot place orders or override Agent 1/2.

`live-proof` builds a capped spot-limit candidate from the latest deterministic `ACTIVE_LIMIT` paper layer and writes:

```text
reports/live_proof_latest.md
reports/live_proof_latest.json
```

It does not place a real order. `live-readiness` gives the full operator checklist: config flags, credential-env presence, operator halt, latest deterministic plan state, proof/preflight, open live order count, and live position count. It writes:

```text
reports/live_readiness_latest.md
reports/live_readiness_latest.json
```

Manual real execution is intentionally separate and requires the exact confirm phrase:

```bash
./bin/btc-agent execute-live-proof-order --config config.yaml --confirm I_UNDERSTAND_THIS_PLACES_A_REAL_SPOT_LIMIT_ORDER
```

Auto live execution is fail-closed. It requires `live.auto_execute=true`, `live.require_manual_confirm=false`, `live.canary_mode=true`, `BTC_AGENT_ALLOW_AUTO_LIVE=true`, operator halt inactive, passing proof/preflight, open live orders below the configured cap, and position budget room. First rollout should use canary max notional and ladder caps (for example `live.canary_max_notional_usdt: 2`, `live.auto_ladder_enabled: true`, `live.max_auto_layers_per_cycle: 1`, `live.max_open_live_orders: 1`, `live.auto_ladder_max_notional_usdt: 2`). Auto ladder still submits only spot post-only BUY limit orders and reconciles after submission.

`reconcile-live-orders` reads local open live orders, checks OKX order status with read-only signed REST calls, updates local order/event state, and writes:

```text
reports/live_reconcile_latest.md
reports/live_reconcile_latest.json
reports/live_position_latest.md
reports/live_position_latest.json
```

Reconciliation never places orders. Unknown exchange errors are marked for manual check. `live-positions` prints the local live position ledger without calling the exchange.

### 24/7 live rollout runbook

1. Keep operator halt active by default:

```bash
./bin/btc-agent operator-halt --config config.yaml
```

2. Run daily plan + readiness checks without placing orders:

```bash
./bin/btc-agent run-daily --config config.yaml
./bin/btc-agent live-readiness --config config.yaml
./bin/btc-agent live-proof --config config.yaml
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent live-positions --config config.yaml
```

3. Manual one-order canary only after readiness is clean:

```bash
./bin/btc-agent operator-resume --config config.yaml
./bin/btc-agent execute-live-proof-order --config config.yaml --confirm I_UNDERSTAND_THIS_PLACES_A_REAL_SPOT_LIMIT_ORDER
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent operator-halt --config config.yaml
```

4. 24/7 scheduler/supervisor mode:

```bash
# Paper/report smoke mode. Scheduler runs with --dry-run.
export BTC_AGENT_MODE=paper
./scripts/btc-agent-scheduler.sh

# Live proof/reconcile visibility only. Scheduler still runs with --dry-run.
export BTC_AGENT_MODE=live-proof
./scripts/btc-agent-scheduler.sh

# Canary auto only after manual live has proven safe.
export BTC_AGENT_MODE=live-canary-auto
export BTC_AGENT_ALLOW_AUTO_LIVE=true
./bin/btc-agent live-doctor --config config.yaml
./scripts/btc-agent-scheduler.sh
```

Legacy loop remains available as `./scripts/btc-agent-24h.sh`, but P2 production runtime should prefer scheduler/supervisor because it uses `live-supervisor`, doctor startup checks, heartbeat reports, and sequential non-overlapping cycles.

Secrets:

- Never paste real exchange or Telegram keys into chat/logs.
- Rotate any key pasted outside the device.
- Use OKX IP whitelist and least permissions where possible.
- If you accept local env-file risk, copy `btc-agent.env.example` to `$HOME/btc-agent.env` and fill values there. Never commit real secrets.

Termux:Boot defaults should use `BTC_AGENT_MODE=paper` unless `$HOME/btc-agent.env` explicitly overrides it.

## Tests

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o bin/btc-agent .
```

## Cron idea

Use Termux:Boot or cronie if installed:

```bash
0 8 * * * ./bin/btc-agent run-daily --config config.yaml
```
