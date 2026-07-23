# Tasks

## Open

- [ ] Re-run current-release production verification after deploying the reviewed SHA; do not infer runtime state from docs.
- [ ] Monitor until deterministic `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` appears; no sizing or authority change while blocked.
- [ ] Use full verification gate before reporting implementation work done.
- [ ] Use `real-data-survey` + `learn` as report-only evidence before future rule tuning.
- [ ] Milestone C: collect enough embargoed evaluation samples, compare false-positive/drawdown against the approved baseline, and obtain manual review before any live sizing change. The report path is implemented; sizing expansion remains disabled.
- [ ] Complete approved shadow/canary observation and retain V1 through rollback window.
- [x] Merge cutover branch after CI and halted shadow verification; main is at `fc6e4c7`.
- [ ] Run V1 cleanup only after approved rollback window and explicit `AGENT_CLEANUP_APPROVED`.
- [ ] Keep production operator halt active; no real-order approval from this release.
- [x] Add authoritative CI checks for Go race/vet/build, secret scan, backup and systemd checks.
- [x] Add report-only liquidation proxy and anchored VWAP/volume profile diagnostics.
- [x] Wire autonomous exits through Hermes-owned, no-short, reconcile-safe limit SELL lifecycle with residual reservation.

## Done

- [x] Add repository hygiene docs and verification gate.
- [x] Keep local config, data, reports, logs, backups, and binaries out of version control through `.gitignore`.
- [x] Harden live-auto readiness and supervisor path.
- [x] Add technical scorecard, opportunity composite score, capital research plan, universe research, and Telegram read-only management commands.
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
- [x] Scheduler chạy monitoring với operator halt là authority runtime; không release, restart hoặc tài liệu nào được tự clear halt. Mọi thay đổi authority phải qua control-plane và audit hiện hành.

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
