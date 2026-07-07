# BTC Agent Upgrade Review

## 1. Current State

- Workdir confirmed: `/data/data/com.termux/files/home/.openclaw/workspace/btc-agent`.
- Requested initial command issue: `find . -maxdepth 2 -type f | sort | sed -n '1,240p'` and `find . -name '*.go' | wc -l` first returned `-S: error while loading shared libraries: -S: cannot open shared object file: No such file or directory` and `0`. Fallback Python listing found 167 Go files and listed tests under `internal`.
- Git latest commits:
  - `a070676 Add Agent2 opportunity audit`
  - `c61f597 Add safe secret setup helpers`
  - `7216dc6 Add repo hygiene verification gate`
  - `4a4ec3d Fix notification and safety correctness gaps`
  - `4fac7bf Add typed scout decision pipeline`
- Working tree before report write was clean. This file is the only intended new file.
- Bot purpose: deterministic BTC market gate plus ETH/SOL/RENDER accumulation planner. README states BTC is benchmark/gate, not accumulation target (`README.md:61-79`).
- Core command router lives in `run` (`main.go:42-147`) and exposes fetch/analyze/plan/status/backtest/learn/live-proof/live-readiness/live-doctor/live supervisor/order-management paths.
- Current local config, redacted:
  - `app.mode=paper` (`config.yaml:2`).
  - Telegram enabled with token/chat redacted (`config.yaml:75-80`).
  - `live.enabled=true`, OKX env names redacted, post-only/manual/proof/canary settings present (`config.yaml:103-138`).
  - `execution.paper_trading=true`, `execution.real_trading_enabled=false` (`config.yaml:139-142`).
  - `live.auto_execute=false`, `live.auto_ladder_enabled=false`, `live.order_management_enabled=false`, `live.supervisor_enabled=false`, `live.proof_only=true` (`config.yaml:113-138`).
- Verification commands run:
  - `go test -v -count=1 ./...`: PASS.
  - `go vet ./...`: PASS.
  - `go build -o bin/btc-agent .`: PASS.
  - `source .env 2>/dev/null || true; ./bin/btc-agent status --config config.yaml`: PASS.
  - `source .env 2>/dev/null || true; ./bin/btc-agent live-proof --config config.yaml`: PASS.
- Current BTC/Agent2 status from command output:
  - BTC `DOWNTREND`, permission `WATCH`, trend score `19.8`, risk `MEDIUM`, falling knife `MEDIUM`, FOMO `LOW`.
  - Flow `NEUTRAL`, score `0.00`.
  - Agent2 state `WATCH`.
  - Watchlist candidates all non-actionable because BTC permission hard checklist fail plus soft waits:
    - SOLUSDT readiness `0.49`, `EARLY_WATCH`.
    - ETHUSDT readiness `0.49`, `EARLY_WATCH`.
    - RENDERUSDT readiness `0.49`, `EARLY_WATCH`.
  - Open paper orders: `0`.
- Live proof status:
  - `NOT_READY_NO_DETERMINISTIC_ORDER`.
  - Reason: `no deterministic ACTIVE_LIMIT layer available`.
  - `No real order was placed.`
  - Account check: `auth_ok=true balance_ok=true base=USDT free_usdt=3275.45 min_required=20.00`.
  - Telegram sent OK for live-proof: `telegram sent ok [live-proof] msg_id=835`.
- Telegram/OKX redacted status:
  - Telegram config present and live-proof send succeeded. Token/chat ID not printed.
  - OKX credential env present enough for auth/balance. Key/secret/passphrase not printed.
- Whether real order can be placed now:
  - No. Local config blocks it: `execution.real_trading_enabled=false`, `live.proof_only=true`, `live.require_manual_confirm=true`, `live.auto_execute=false`, `live.order_management_enabled=false`, and no deterministic `ACTIVE_LIMIT` exists.
  - Manual execution additionally requires exact confirm phrase and inactive operator halt in `manualOrderBlockers` (`internal/liveguard/executor.go:151-213`).
  - Auto execution additionally requires `BTC_AGENT_ALLOW_AUTO_LIVE=true`, `live.auto_execute=true`, `live.proof_only=false`, `execution.real_trading_enabled=true`, canary mode, no blockers (`main.go:800-823`, `internal/liveguard/executor.go:265-333`).

## 2. Strengths

- Safety-first config validation:
  - `Config.Validate` blocks unsafe risk flags if futures/leverage/market-order protections are off (`internal/config/config.go:317-319`).
  - Real trading cannot validate unless `live.enabled=true` and `live.proof_only=false` (`internal/config/config.go:204-207`).
  - Auto execution requires manual confirm off and canary mode on (`internal/config/config.go:208-214`).
  - Manual live execution requires manual confirm if auto execute is not enabled (`internal/config/config.go:215-217`).
- Deterministic state authority:
  - Agent2 states are explicit: `NO_TRADE`, `WATCH`, `SCOUT`, `ARMED`, `ACTIVE_LIMIT` (`internal/agent2/planner.go:15-23`).
  - `OrdersFromPlan` only emits orders for assets in `StateActiveLimit` (`internal/agent2/paper_trading.go:19-30`).
  - Live proof only considers `plan.State == StateActiveLimit` and asset `StateActiveLimit` (`internal/liveguard/guard.go:213-233`).
- BTC gate protects execution:
  - BTC hard reasons include panic, high falling knife, high FOMO (`internal/agent2/planner.go:139-156`).
  - Non-ALLOWED BTC permission is soft wait and downgrades `ACTIVE_LIMIT` asset plans to `SCOUT`/`ARMED`, removing or shrinking layers (`internal/agent2/planner.go:163-199`).
- Asset setup has multiple independent gates:
  - Relative strength compares asset vs BTC with lookback and momentum thresholds (`internal/agent2/relative_strength_filter.go:17-44`).
  - Rotation scoring combines relative strength, momentum, discount quality, and flow (`internal/agent2/rotation_score.go:27-80`).
  - Asset flow entry uses MM accumulation signals and can hard-block confirmed bearish flow (`internal/agent2/flow_entry_filter.go:46-72`).
  - Watchlist checklist makes hard vs soft blockers visible (`internal/agent2/watchlist.go:268-383`).
- Live proof does not place orders:
  - `BuildProofWithChecks` builds proof and preflight only (`internal/liveguard/guard.go:157-210`).
  - `runLiveProof` writes reports and Telegram, then prints `No real order was placed` (`main.go:825-873`).
- OKX safety basics are correct:
  - Spot order uses `tdMode: cash` and `ordType: post_only` when post-only set (`internal/exchange/live/okx.go:78-93`).
  - Instrument filters, tick/step rounding, min size, min notional, canary/max notional enforced in preflight (`internal/liveguard/preflight.go:31-95`).
  - OKX response/body redaction truncates and replaces secrets (`internal/exchange/live/okx.go:375-386`, `internal/liveguard/executor.go:224-250`).
- Operator halt is fail-closed:
  - SQLite migration defaults `operator_settings halted=true` (`internal/storage/sqlite.go:40-54`).
  - `IsHalted` returns true on missing row or DB error (`internal/storage/sqlite.go:544-553`).
  - Manual/auto/live manager blockers treat halted/unknown as blocked (`internal/liveguard/executor.go:151-163`, `internal/liveguard/order_manager.go:112-129`).
- Order manager already has useful model objects:
  - Desired/decision/cycle/per-coin structs include layer, source, invalidation, quality, allocation, target, RR, expiry, reasons (`internal/liveguard/order_manager.go:31-106`).
  - Manager can keep/cancel/replace/place and dry-run (`internal/liveguard/order_manager.go:108-160`).
- Storage has live ledger foundations:
  - Tables for live orders, events, fills, positions, position events exist (`internal/storage/sqlite.go:48-53`).
  - Reconcile updates order status and live position ledger incrementally (`main.go:2337-2448`, `internal/storage/sqlite.go:358-484`).
- Telegram is non-fatal:
  - `sendTelegram` logs config/send warnings and returns without failing command (`main.go:2221-2234`).
  - Telegram chunking and token redaction exist (`internal/notify/telegram.go:32-103`).
- Backtest/audit breadth is strong:
  - `backtest.Result` includes data sanity, BTC flow, permission, threshold, zone, MM, Agent2 sim, watchlist, near-miss, flow entry, checklist, opportunity, layer, exit audits (`internal/backtest/backtest.go:29-59`).
  - Agent2 simulation avoids same-candle fills by using `activeFromIndex=index+1` (`internal/backtest/agent2_sim.go:309-332`).
  - Backtest documents local/OHLCV limitations in notes (`internal/backtest/agent2_sim.go:207-219`).

## 3. Weaknesses / Gaps

- Config safety:
  - `config.yaml.example` is safer than local config, but README still says "No real order executor in phase 1" while code now has manual/auto/live manager paths (`README.md:4-9`, `README.md:215-260`). Docs are stale and can mislead operators.
  - `live.enabled=true` in local `config.yaml` while `proof_only=true` and real trading false. Safe, but can confuse because OKX auth is used during proof/readiness.
  - `config.Validate` limits `live.max_order_notional_usdt` to <=10 only when `execution.real_trading_enabled=true` (`internal/config/config.go:204-220`). In proof-only mode, oversized live caps could be configured without validation.
  - Liquidity gate fields exist in config struct (`internal/config/config.go:147-153`) but are absent from `config.yaml.example`, so defaults/intent are not obvious.
- Telegram reliability:
  - Chunking splits exactly at 4000 runes and does not prefer line boundaries (`internal/notify/telegram.go:78-93`). Multi-line tables can split badly.
  - `TelegramDelete` and `TelegramEdit` do not redact token or response body consistently; delete returns raw body on non-2xx (`internal/notify/telegram.go:105-150`).
  - No retry/backoff for Telegram 429/5xx; one transient failure drops notification (`internal/notify/telegram.go:32-75`).
- OKX/live guard:
  - `NewOKXFromEnv` reads env values raw. Previous malformed env with whitespace/control chars caused header errors; code does not trim/reject control chars before building headers (`internal/exchange/live/okx.go:29-51`, headers at `internal/exchange/live/okx.go:103-107`).
  - `runLiveProof` silently disables account/filter readers if OKX client creation fails, then proof reports config missing env but not the client creation reason (`main.go:830-839`, `internal/liveguard/guard.go:174-178`). Fine for secrecy, less useful for diagnosis.
  - Manual order flow can create OKX client before blocking on confirm/proof-only/real-trading flags (`main.go:1381-1396`). Safety still blocks before placing, but it can touch credential env unnecessarily.
- Allocation logic:
  - Planner budget uses static portfolio allocation and max deployment (`internal/agent2/planner.go:259-263`), while live manager later says opportunity allocation follows setup score (`main.go:1249-1252`). This split is hard to reason about.
  - `desiredLiquidityNotional` prioritizes live cap/canary cap over portfolio budget (`internal/agent2/planner.go:421-435`); useful for small live, but can understate liquidity needs relative to paper/backtest budget.
- Order lifecycle:
  - Paper orders are persisted as `OPEN` but no normal paper lifecycle updates to FILLED/EXPIRED/CANCELLED in `OpenPaperOrders`/`SaveOrders` path (`internal/storage/sqlite.go:195-223`). Backtest sim has lifecycle, production paper storage does not.
  - `OrdersFromPlan` ID uses timestamp seconds and symbol/layer (`internal/agent2/paper_trading.go:19-30`). Multiple plan runs within one second can produce same ID and `INSERT OR REPLACE` old rows (`internal/storage/sqlite.go:195-203`).
  - `SaveOrders` uses `INSERT OR REPLACE`, so duplicate paper order IDs overwrite prior order state (`internal/storage/sqlite.go:195-203`).
  - Managed live `ExpiresAt` persistence uses `now` instead of desired expiry (`main.go:1876-1878`), so stale-order logic may not reflect original layer expiry.
- Storage/state:
  - Many writes outside transactions (`SaveOrders`, `SavePlan`, reports) can leave plan/order/report partially updated if a later write fails (`internal/storage/sqlite.go:190-203`, `main.go:294-317`).
  - `SaveLiveOrderStatus` only updates existing rows; unknown remote orders become manual check instead of safely importing into a quarantine table (`internal/storage/sqlite.go:302-346`, `main.go:2403-2448`).
  - No schema version table; migrations are implicit table/column creation (`internal/storage/sqlite.go:39-76`).
- Backtest realism:
  - Agent2 historical sim uses BTC 1D candles as fallback for 4H/1W, explicitly not true multi-timeframe alignment (`internal/backtest/agent2_sim.go:207-219`).
  - Limit fill model fills when candle low crosses price with no queue/slippage/order book/partial fill model (`internal/backtest/agent2_sim.go:342-380`).
  - Invalidation can occur on same candle after fill because OHLCV ordering is unknown; code has conservative logic in some audits, but sim remains approximate (`internal/backtest/agent2_sim.go:383-416`).
- Alerting:
  - Telegram is sent for daily/live-proof/readiness/reconcile/supervisor, but no unified alert taxonomy for plan-created, order-submitted, partial-fill, filled, cancel, reject, expired, daily summary.
  - Notification policy is spread in command functions rather than central alert router (`main.go:356-365`, `main.go:868-870`, `main.go:1762-1775`, `main.go:2394-2396`).
- Observability/logging:
  - Logs use `log.Printf` free text; no structured event IDs/correlation IDs for a cycle/order (`main.go` many functions).
  - Reports exist, but live manager/report filenames can alias legacy and managed concepts (`writeAutoLiveManagementResult` writes both `auto_live_management_latest.*` and `auto_live_ladder_latest.*`, `main.go:1829-1854`).
- Duplicate order protection:
  - Live client order ID uniqueness is atomic per process (`internal/liveguard/executor.go:215-263`), but not persisted across restarts. Timestamp/nano collision is unlikely, but no DB uniqueness pre-reservation before network submit.
  - Manager key is symbol+layer (`internal/liveguard/order_manager.go:648`) and fallback match by price exists, but partial submit failure after network success and before DB save remains a risk in `persistManagedCycleResult` (`main.go:1856-1885`).
- Autonomous mode readiness:
  - Strong gates exist, but autonomous mode is not ready for unattended capital because current local settings disable it and current market has no `ACTIVE_LIMIT`.
  - `runDaily` still contains standalone auto-live branch if supervisor disabled (`main.go:343-355`). It calls `requireAutoLiveRuntime`, but long-term one authority path would be safer.

## 4. Bugs Found

### Bug 1: Initial requested `find` commands failed in this shell

- File: environment/tooling, not project source.
- Function: shell command execution.
- Exact issue: `find . -maxdepth 2 -type f | sort | sed -n '1,240p'` and `find . -name '*.go' | wc -l` returned `-S: error while loading shared libraries: -S: cannot open shared object file: No such file or directory` and Go file count `0`.
- Risk: false audit result if trusted literally.
- Suggested fix: use absolute/system coreutils if Termux environment has broken aliases/wrappers, or use Go/Python fallback for file enumeration in scripts.

### Bug 2: Paper order ID can collide within one second

- File: `internal/agent2/paper_trading.go:19-30`; `internal/storage/sqlite.go:195-203`.
- Function: `OrdersFromPlan`, `SaveOrders`.
- Exact issue: ID format is `YYYYMMDDHHMMSS-SYMBOL-L<index>`. Re-running `plan` twice within same second for same symbol/layer produces same ID. DB uses `INSERT OR REPLACE`, overwriting existing paper order.
- Risk: paper order history/state loss; duplicate prevention appears stronger than it is.
- Suggested fix: include monotonic nanosecond suffix or deterministic plan ID + layer ID, and use insert-only with explicit duplicate handling instead of replace.

### Bug 3: Paper order lifecycle is incomplete in production path

- File: `internal/storage/sqlite.go:195-223`; `main.go:294-317`.
- Function: `SaveOrders`, `OpenPaperOrders`, `plan`.
- Exact issue: paper orders are created and stored as `OPEN`; no regular command updates them to FILLED, EXPIRED, CANCELLED, or REPLACED outside backtest simulation.
- Risk: stale open paper orders accumulate and status command can misrepresent current paper state.
- Suggested fix: add `paper-manager` command to process latest candles against open paper orders, handle expiry/cancel/fill/invalidation, and report lifecycle transitions.

### Bug 4: Managed live order expiry persisted as now

- File: `main.go:1876-1878`.
- Function: `persistManagedCycleResult`.
- Exact issue: `meta := live.OrderStatus{..., ExpiresAt: now}` uses current time, not `desired.ExpiresAt.Unix()`.
- Risk: stale/cancel logic using `expires_at` can treat just-placed managed orders as expired or lose intended expiry semantics.
- Suggested fix: set `ExpiresAt` from `desired.ExpiresAt.Unix()` when non-zero; add unit test asserting saved metadata expiry matches desired layer expiry.

### Bug 5: OKX env values not normalized/rejected before header use

- File: `internal/exchange/live/okx.go:29-51`, headers at `internal/exchange/live/okx.go:103-107` and `175-187`.
- Function: `NewOKXFromEnv`, `PlaceSpotLimitOrder`, `AccountBalance`.
- Exact issue: env values are read raw; whitespace/control chars can enter HTTP headers and produce invalid header errors.
- Risk: confusing failures; possible secret leakage if low-level error includes bad value shape.
- Suggested fix: validate credentials contain no `\r`, `\n`, NUL, or leading/trailing whitespace; return sanitized config error naming env var only.

### Bug 6: Telegram delete error can include unredacted body

- File: `internal/notify/telegram.go:105-150`.
- Function: `TelegramDelete`, `TelegramEdit`.
- Exact issue: `TelegramDelete` returns raw response body on non-2xx; unlike send path, it does not call `telegramRedact`. `TelegramEdit` drops body but lacks token redaction in any detailed future error.
- Risk: Telegram API error body usually safe, but inconsistent redaction policy and possible token/body leakage if API echoes request context.
- Suggested fix: use `telegramRedact` for delete/edit non-2xx, truncate, and add tests.

### Bug 7: README safety section stale

- File: `README.md:4-9`, `README.md:215-260`.
- Function: docs.
- Exact issue: top safety says `No real order executor in phase 1`, but code has manual execution, auto proof, auto ladder, managed live order, cancel-all commands.
- Risk: operator misunderstands current code capabilities.
- Suggested fix: update top safety to say real execution code exists but is disabled by default and fail-closed behind proof/manual/canary/env/operator gates.

## 5. Upgrade Proposal

### Phase 1: Stability + Safety

Small fixes only. No real trading.

- Fix paper order ID collision:
  - Add nanosecond/monotonic suffix to `OrdersFromPlan` IDs (`internal/agent2/paper_trading.go:19-30`).
  - Change `SaveOrders` away from blind `INSERT OR REPLACE`, or preserve existing non-open statuses.
  - Tests: ID uniqueness under same-second mocked timestamp; no overwrite of FILLED/CANCELLED/EXPIRED paper order.
- Fix managed live expiry metadata:
  - Use `desired.ExpiresAt.Unix()` in `persistManagedCycleResult` (`main.go:1856-1885`).
  - Tests: managed saved live order has intended expiry.
- Harden OKX env validation:
  - Reject whitespace/control chars in `NewOKXFromEnv` (`internal/exchange/live/okx.go:29-51`).
  - Tests: bad env returns sanitized error with env name only, no secret value.
- Harden Telegram delete/edit redaction:
  - Apply `telegramRedact` to delete/edit errors (`internal/notify/telegram.go:105-150`).
  - Tests: token not present in returned error.
- Update README safety wording to match live executor existence and disabled defaults.
- Keep `real_trading_enabled=false`, `proof_only=true`, `auto_execute=false`; do not submit/cancel any real order.

### Phase 2: Paper Order Manager

Improve paper lifecycle only.

- Add `paper-manager` command or fold into `status`/`run-daily` with no live authority.
- State machine: `OPEN`, `FILLED`, `CANCELLED`, `EXPIRED`, `INVALIDATED` for paper orders.
- Use latest 1D/4H candles to update open paper orders:
  - Fill if candle low <= limit price after placement time.
  - Expire after `execution.order_expiry_hours`.
  - Cancel if plan no longer `ACTIVE_LIMIT` or BTC gate turns hard block.
  - Invalidate if candle low <= invalidation.
- Add duplicate prevention:
  - Unique key: symbol + layer + plan timestamp/zone + price bucket.
  - Do not create new paper order if equivalent open order exists.
- Reporting:
  - `reports/paper_manager_latest.md/json`.
  - Status shows open/filled/expired/cancelled counts and stale orders.
- Tests:
  - fill after next candle only.
  - expire works.
  - duplicate plan run does not duplicate or overwrite.
  - inactive plan cancels/rejects new paper order.

### Phase 3: Live Order Manager, disabled by default

Add/standardize live order state machine:

- States: `PLANNED`, `SUBMITTED`, `PARTIAL_FILL`, `FILLED`, `CANCELLED`, `EXPIRED`, `REJECTED`.
- Must remain disabled unless explicitly enabled:
  - `execution.real_trading_enabled=true`.
  - `live.proof_only=false`.
  - `live.auto_execute=true` for auto manager.
  - `live.order_management_enabled=true`.
  - `BTC_AGENT_ALLOW_AUTO_LIVE=true`.
  - `live.canary_mode=true` during rollout.
  - operator halt inactive.
- Spot limit only, no futures, no leverage.
- Pre-reserve client order IDs in DB before network submit; update to SUBMITTED/REJECTED after response.
- Persist exchange status transitions idempotently from reconcile.
- Add emergency cancel path tests with fake OKX only; do not call real exchange in tests.
- Keep manual proof-order path separate and gated by exact phrase.

### Phase 4: Strategy Intelligence

Improve decision quality without giving AI authority.

- BTC gate:
  - Add explicit BTC gate audit output to status: trend threshold gap, RR proxy, flow promotion gap.
  - Tune only from backtest evidence; defaults unchanged.
- Multi-timeframe flow:
  - Ensure historical sim uses actual 4H/1W alignment instead of 1D fallback.
  - Add tests for time alignment and no lookahead.
- Entry/exit:
  - Convert Opportunity Audit into per-symbol closest-unlock report with threshold gap numbers.
  - Add exit planner for take-profit/time-stop research first; no live TP until tested.
- Adaptive sizing:
  - Keep static max caps, but compute suggested paper/live canary notional from setup score, liquidity, regime, history quality.
  - Never increase above configured caps.

### Phase 5: Monitoring + Telegram

Create central alert router.

- Events:
  - plan generated.
  - ACTIVE_LIMIT appeared.
  - proof ready/not ready changed.
  - order planned/submitted/partial-fill/filled/cancelled/expired/rejected.
  - reconcile unknown/manual-check.
  - daily summary.
  - data health block.
  - operator halt/resume.
- Reliability:
  - Retry Telegram 429/5xx with small backoff.
  - Chunk on line boundaries.
  - Redact tokens/OKX/env values everywhere.
  - Store Telegram message IDs per alert type for edit/update where useful.
- Tests:
  - long reports chunk safely.
  - token not leaked in send/delete/edit errors.
  - alert dedupe prevents spam across scheduler cycles.

### Phase 6: Backtest + Simulation

Make simulation closer to live constraints.

- Fake OKX:
  - Instrument filters, order book snapshots, balances, partial fills, cancels, rejects.
  - Use same liveguard manager against fake OKX.
- Walk-forward:
  - Train/tune profiles on one window, evaluate on later window.
  - Report sample counts and confidence.
- Slippage/fee/partial fills:
  - Model post-only maker fee, queue fill probability, min notional/lot size, partial fills.
  - Simulate order book depth assumptions.
- OHLCV caveats:
  - If high and low both cross TP/invalidation same candle, use conservative ordering or configurable assumption with explicit report.
- Tests:
  - no same-candle lookahead.
  - partial fill accounting matches ledger.
  - fake OKX rejects malformed or over-cap orders.

## 6. Recommended Next Tasks

### Prompt A: safe fixes

```text
Workdir: /data/data/com.termux/files/home/.openclaw/workspace/btc-agent

Implement Phase 1 safe fixes only. Do not enable real trading. Do not place orders. Do not print secrets.

Tasks:
1. Fix paper order ID collision in internal/agent2/paper_trading.go and storage SaveOrders overwrite behavior.
2. Fix managed live order ExpiresAt persistence in main.go:persistManagedCycleResult to use desired.ExpiresAt.
3. Validate OKX env values in internal/exchange/live/okx.go: reject whitespace/control chars with sanitized env-name-only errors.
4. Redact Telegram delete/edit errors in internal/notify/telegram.go.
5. Update README safety section to state live executor exists but is disabled/fail-closed by default.

Add/adjust tests for all fixes.
Run: gofmt, go test -v -count=1 ./..., go vet ./..., go build -o bin/btc-agent ., source .env 2>/dev/null || true; ./bin/btc-agent status --config config.yaml; source .env 2>/dev/null || true; ./bin/btc-agent live-proof --config config.yaml.
Report changed files and exact PASS/FAIL. No real order.
```

### Prompt B: paper order manager

```text
Workdir: /data/data/com.termux/files/home/.openclaw/workspace/btc-agent

Implement paper-only order manager. No live trading. No OKX order placement. Do not print secrets.

Add a paper order lifecycle manager with states OPEN, FILLED, CANCELLED, EXPIRED, INVALIDATED. It should process open paper orders using stored candles, expire by execution.order_expiry_hours, avoid duplicate equivalent orders, cancel orders when plan is not ACTIVE_LIMIT or BTC hard blocks, and write reports/paper_manager_latest.md/json. Add CLI command paper-manager and include summary in status.

Add tests for fill-after-next-candle, expiry, duplicate prevention, inactive-plan cancel, invalidation. Keep OrdersFromPlan authority limited to ACTIVE_LIMIT.
Run full verification.
```

### Prompt C: live order manager disabled-by-default

```text
Workdir: /data/data/com.termux/files/home/.openclaw/workspace/btc-agent

Design and implement disabled-by-default live order state machine. Do not enable real trading in config. Do not place real orders. Use fake OKX tests only.

Add states PLANNED, SUBMITTED, PARTIAL_FILL, FILLED, CANCELLED, EXPIRED, REJECTED. Pre-reserve client order IDs in SQLite before network submit. Make submit/reconcile/cancel idempotent. Preserve spot-limit-only, post-only, cash mode, no futures, no leverage. Keep gates: real_trading_enabled, proof_only=false, auto_execute, order_management_enabled, BTC_AGENT_ALLOW_AUTO_LIVE, canary_mode, operator halt inactive.

Add fake OKX tests for submit success, reject, partial fill, full fill, cancel, duplicate retry after crash. Run full verification. No real exchange order.
```

### Prompt D: monitoring/Telegram

```text
Workdir: /data/data/com.termux/files/home/.openclaw/workspace/btc-agent

Implement central monitoring/Telegram alert router. Do not change trading behavior. Do not print secrets.

Add alert event types for plan, ACTIVE_LIMIT, proof status change, submit, partial fill, fill, cancel, expire, reject, reconcile unknown, data health block, operator halt/resume, daily summary. Add line-boundary chunking, retry/backoff for 429/5xx, dedupe/rate limit, full token/secret redaction. Store Telegram message IDs where edit/update is useful.

Add unit tests with fake Telegram server. Run full verification. No real order.
```

### Prompt E: backtest realism

```text
Workdir: /data/data/com.termux/files/home/.openclaw/workspace/btc-agent

Upgrade backtest realism without changing production trading behavior. No real trading. No orders.

Implement fake OKX simulator with filters, balances, order book assumptions, maker fee, slippage, partial fills, cancel/reject paths. Make Agent2/live manager historical simulation use actual 4H/1W alignment where available instead of 1D fallback. Add walk-forward split reports and conservative OHLCV ordering for fill/TP/invalidation ambiguity.

Add tests for no lookahead, partial fill ledger, fake OKX rejects, min notional/lot/tick handling, walk-forward split. Run full verification.
```

## 7. Do Not Do Yet

- Do not set `execution.real_trading_enabled=true`.
- Do not set `live.proof_only=false`.
- Do not enable auto live trading.
- Do not remove manual confirm.
- Do not disable operator halt as part of code work.
- Do not place or cancel any real order.
- Do not add market orders.
- Do not add futures.
- Do not add leverage.
- Do not bypass `ACTIVE_LIMIT` as order authority.
- Do not let `WATCH`, `SCOUT`, or `ARMED` create real orders.
- Do not loosen BTC permission/risk gates without backtest evidence and separate approval.
- Do not commit or print `.env`, `config.yaml` secrets, OKX keys, Telegram token, logs with secrets, DB files, reports, or binaries.
