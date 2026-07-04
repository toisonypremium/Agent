# btc-agent

Rule-based BTC market analyst + ETH/SOL/RENDER accumulation planner for Termux/root Android.

## Safety

- Default is paper trading/simulation.
- No futures, no leverage.
- No real order executor in phase 1.
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
./bin/btc-agent run-daily --config config.yaml
./bin/btc-agent run-ai-watch --config config.yaml
./bin/btc-agent status --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent export-training --config config.yaml
./bin/btc-agent eval-ai --config config.yaml
./bin/btc-agent live-proof --config config.yaml
./bin/btc-agent maintenance --config config.yaml
```

## Telegram/ntfy

Edit `config.yaml` notify section. Leave disabled unless configured.

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

`maintenance` prunes old report rows, old live event rows, closed paper-order history over the configured cap, and old unprotected files in `reports/`. It does not prune candles, live orders, fills, positions, or operator halt settings.

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

Use this to audit Liquidity Flow signals by historical forward returns and drawdowns. It also runs BTC Flow Detector Bottleneck Audit, which reports flow component frequency, bias forward return/drawdown, and audit-only parameter sensitivity; it does not tune `flow.DefaultParams()` automatically. Flow Param Candidate Forward Quality Audit then compares those audit-only param candidates by bullish/bearish forward return, added signal quality, false-positive proxy, and drawdown; it also does not tune production params automatically. It also runs BTC Permission Bottleneck Audit, which reports Agent 1 permission distribution, forward return/drawdown by permission, and top blockers explaining why BTC is not `ALLOWED`; this is diagnostic only and does not tune Agent 1 automatically. It also runs Agent 2 Layer Simulation for ETH/SOL/RENDER using local 1D candles: limit-layer placement, fills, expiries, invalidation hits, max deployed capital, drawdown, and simulated PnL. Agent 2 diagnostics explain Agent 1 permission counts, regime/risk gates, per-asset block reasons, and sample PLAN/FILL/EXPIRE/INVALIDATION/TAKE_PROFIT/TIME_STOP events. Orders become active from the next candle to avoid same-candle lookahead. The Layer Audit compares invalidation buffers and layer-depth multipliers to see whether volatile assets like RENDER fail because stops are too tight or layers are too shallow. The Exit / Take-Profit Audit compares TP percentages and time-stop windows using local candles only; if TP and invalidation happen in the same candle, invalidation wins as the conservative OHLCV assumption. Agent 2 also uses an Asset Relative Strength Filter when BTC 1D benchmark candles are available: assets that are both weak in absolute momentum and underperforming BTC over the configured lookback are blocked before layer planning. Agent 2 then reports Asset Ranking / Rotation Score, combining relative strength, momentum, discount quality, and asset-level liquidity flow; low-score or low-rank assets stay WATCH before layer planning. Agent 2 also requires Asset Flow Entry confirmation before layer planning: sweep-low reclaim, failed breakdown/bear trap, absorption, or accumulation near support can pass; distribution, bull-trap, or failed breakout hard-blocks entry. Candidate Discovery / Watchlist Report appears in `plan`, `status`, and `reports/latest.json`; it ranks the closest candidates by readiness, lists missing conditions, gives the next trigger to wait for, and includes a Strict Entry Checklist for BTC permission, relative strength, rotation score/rank, asset flow entry, discount zone, reward/risk, falling knife, and FOMO. Backtest also includes Watchlist Trigger Audit, which measures forward return, win rate, and worst drawdown after historical actionable watchlist candidates appear, plus Checklist Pass-Count Audit, which reports average checks passed, hard/soft fail rates, near-actionable samples, and top blockers per asset. Watchlist readiness is noise-capped: BTC-not-allowed, relative-weak, falling-knife, FOMO, and unconfirmed-flow candidates remain visible for context but are not actionable by default. Ranking, flow entry, watchlist reporting, Strict Entry Checklist, Checklist Pass-Count Audit, BTC Flow Detector Bottleneck Audit, Flow Param Candidate Forward Quality Audit, BTC Permission Bottleneck Audit, and trigger audit do not reallocate capital, trigger alerts, tune rules, or enable real trading. Backtest diagnostics count these blocks in per-asset reasons. These audits do not change production planner/config automatically and do not enable take-profit order placement. They are for debugging rules, not a profit guarantee. If the sample is small, treat the conclusion as weak.

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

It does not place a real order. Real order execution is intentionally a separate future manual-confirmed command.

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
