# Tasks

## Open

- [ ] Monitor until deterministic `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` appears (BTC currently in MARKDOWN phase).
- [ ] Use full verification gate before reporting implementation work done.
- [ ] Use `real-data-survey` + `learn` as report-only evidence before future rule tuning.
- [ ] Milestone C: prove BTC accumulation false-positive/drawdown improvement in walk-forward before any live sizing change.
- [ ] Wire `PlaceSellLimitOrder` when operator decides to enable auto exit execution (currently report-only).
- [ ] Keep Hermes in `observe`/`shadow`; promote to canary only after explicit operator review and a fresh `hermes-canary-readiness` READY report.

## Phase A safety hardening — done

- [x] Preserve the managed first-order dry-run approval/history context through the Hermes execution bridge.
- [x] Require `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` for every Hermes exposure-increasing BUY, including `PROBE_LIMIT`.
- [x] Enforce a fresh canary readiness report at the production boundary before canary exposure increases.
- [x] Replace date-hardcoded lifecycle qualification lookup with latest valid artifact discovery plus SHA-256 provenance and freshness checks.
- [x] Add regression tests for missing/valid dry-run approval, probe gate blocking, stale readiness, and dry-run zero exchange calls.

## Done

- [x] Add repository hygiene docs and verification gate.
- [x] Keep local config, data, reports, logs, backups, and binaries out of version control through `.gitignore`.
- [x] Harden live-auto readiness and supervisor path.
- [x] Add technical scorecard, opportunity composite score, capital research plan, universe research, decision dashboard, and Telegram read-only management commands.
- [x] Connect live allocator to `OpportunityComposite` inside `ACTIVE_LIMIT` guard.
- [x] Remove stale canary/auto-ladder production logic from new live-auto scenario.
- [x] Add real-data survey report path for learning evidence without changing live authority.
- [x] Add OHLCV BTC accumulation phase detector and false-positive/forward-return audit without changing live authority.
- [x] Add auto-live market-watch monitoring and operations-plan report without changing live authority.
- [x] Split overloaded command/scheduler code and add read-only runtime ops event queue without changing live authority.
- [x] Add report-only microstructure data sources and stale blockers without changing live authority.
- [x] Milestone B: add microstructure data sources (CVD/OI/funding/orderbook) with data-health stale blockers before considering live expansion.
- [x] Add live-auto safety hardening before autonomous real order approval.
- [x] Add pre-live safety hardening: final execution assertion, live-auto-audit, forced dry-run simulation, first-order quarantine, and near-unlock events.
- [x] Fix live-auto safety-hardening logic deviations: dry-run proof gate, BTC phase final assertion, audit verdict separation, forced simulation exchange counter, and near-unlock alert lifecycle.
- [x] Add exit manager: EvaluateExits, PeakTracker, ExitPanicSell, wire into supervisor cycle, tests (18 cases).
- [x] Add OpenedAt to LivePosition for accurate time-stop tracking.
- [x] Schedule live-auto-audit in scheduler loop (audit_interval_minutes, default 60 min).
- [x] Clear operator halt; bot monitoring active, order gates BLOCKED pending ACCUMULATION_CONFIRMED.

## Verification commands

```bash
gofmt -w .
go test -v -count=1 ./...
go vet ./...
go build -o bin/btc-agent .
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true ./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent live-doctor --config config.yaml
```

## Safety invariants

- Scheduler `live-auto` uses supervisor + managed order engine as production path.
- `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` required for normal live desired orders.
- `WATCH`, `SCOUT`, and `ARMED` do not create normal live orders.
- Spot limit BUY post-only only.
- No futures.
- No leverage.
- No market order.
- Telegram remains read-only.
- `config.yaml`, `.env`, DB, reports, logs, backups, and binaries stay local-only.
