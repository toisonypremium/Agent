# Architecture Audit — btc-agent V1

## Scope and baseline

Repository: `toisonypremium/Agent`, source under `btc-agent/`, commit
`7fffba8c8a02f566da98f7e63285cc4197d88181`, branch `architecture-v2`.

- 189 Go files, 79 test files, 27 internal packages.
- `main.go`: 2,777 lines; CLI router, composition root, orchestration, persistence,
  reporting and live command handling.
- `scheduler.go`: 789 lines; scheduler, PID lock, heartbeat and orchestration.
- Largest risk modules: `internal/backtest`, `internal/liveguard`,
  `internal/storage/sqlite.go`, `internal/exchange/live/okx.go`.
- Baseline on Termux Android arm64, Go 1.26.3:
  - `go test -count=1 ./...`: pass.
  - `go vet ./...`: pass.
  - `go build -o bin/btc-agent .`: pass.
  - No real order was submitted.

## Current architecture and flow

```text
CLI main.go
  -> config.Load + Config.Validate
  -> storage.Open + implicit Migrate
  -> command handler
  -> Binance candles / research / OKX readers
  -> Agent1 analysis -> Agent2 plan
  -> paper/live guard and manager
  -> SQLite reports/ledger -> Telegram notification
```

```text
scheduler
  -> PID lock -> heartbeat -> research/daily/reconcile/supervisor/maintenance
  -> graceful shutdown on SIGINT/SIGTERM
```

```text
plan
  -> proof/preflight/data-health/risk checks
  -> operator halt and config gates
  -> liveguard/order manager
  -> SQLite reservation/state -> OKX spot limit adapter
  -> reconcile remote orders/fills/positions
```

Current source of truth: OKX for exchange state; Binance for candles; SQLite for
nearly all durable application state; Telegram for notifications. Supabase, R2,
Vercel and dashboard do not exist in the current repository.

## Strengths to preserve

- Fail-closed config validation; paper/proof-only defaults; no futures/leverage.
- Operator halt defaults to `true`; read errors block execution.
- BTC gate and Agent2 state machine remain decision authority.
- Live preflight validates precision, min size/notional, post-only and caps.
- Live manager has lifecycle/reconcile/reservation and dry-run paths.
- Simulator, fake OKX and broad backtest/test coverage.
- Scheduler has graceful shutdown, heartbeat and basic process lock.
- Baseline test, vet and build are green.

## Technical debt and coupling

1. `main.go` combines CLI, dependency wiring, use-case orchestration, storage,
   reporting, notifications and live execution.
2. `scheduler.go` combines runtime loop with research, reconcile, Telegram and
   supervisor calls.
3. SQLite combines schema, implicit migrations, queries, serialization and state
   transitions; no schema version/history exists.
4. No stable application-service boundary or dependency-inversion layer.
5. Logging is mostly free-form and lacks consistent correlation/decision/order IDs.
6. Notification policy is distributed; no central alert router, retry or dedupe.
7. No durable outbox, Supabase/R2 adapters, dashboard, systemd release layout or
   rollback automation.
8. Backtest has timeframe fallback and approximate limit-fill behavior.

## Live-trading and operational risks

- PID lock protects one scheduler host only; no lease/fencing for V1/V2 or instances.
- Client-order reservation is not unified across every network-submit path.
- Network success before persistence can leave uncertain order state; quarantine and
  reconcile are required.
- `runDaily` still has an auto-live branch outside the supervisor authority path.
- No dashboard approval/challenge workflow or actor-level command audit yet.
- OKX environment values need whitespace/control-character validation.
- Multi-step cycle writes can leave partial state on failure.
- Unknown remote orders may become manual-check instead of controlled quarantine.
- Heartbeat lacks complete owner, Git SHA, outbox and cloud-sync fields.
- VPS deployment is not release-based and lacks hardened systemd/rollback scripts.

## Keep, refactor, replace

### Keep and wrap

Market/indicator/flow/Agent1/Agent2 logic, liveguard safety rules and preflight,
simulator/fake exchange and existing fixtures.

### Refactor incrementally

Split `main.go` into `cmd/agent` and application services; split scheduler into
runtime loop, heartbeat, ownership and use-case runner; split SQLite into versioned
migrations, repositories, outbox and recovery; standardize exchange/notification/
LLM interfaces; add structured events and IDs.

### Build new

Execution lease/fencing, durable local outbox, Supabase migrations/RLS/read model,
R2 adapter, read-only dashboard, VPS release/systemd/backup/rollback/health scripts,
shadow comparison and cutover runbook.

### Remove only after cutover

Legacy scheduler/duplicate command paths and long-term SQLite history, only after
shadow, backup, synchronization and rollback are proven.

## Phase 0 conclusion

Do not perform a big-bang rewrite. V1 has useful safety foundations but is not ready
for unattended live trading or cloud migration. Characterization tests must lock BTC
gate, decision/plan/order/reconciliation behavior before composition-root and runtime
refactoring. Supabase, R2 and Vercel must remain outside the critical execution path.
