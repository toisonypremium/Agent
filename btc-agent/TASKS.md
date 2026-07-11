# Tasks

## Open

- [ ] Keep scheduler running in real `live-auto` (`dry_run=false`) after verification.
- [ ] Monitor until deterministic `ACTIVE_LIMIT + ALLOWED` appears.
- [ ] Use full verification gate before reporting implementation work done.
- [ ] Use `real-data-survey` + `learn` as report-only evidence before future rule tuning.

## Done

- [x] Add repository hygiene docs and verification gate.
- [x] Keep local config, data, reports, logs, backups, and binaries out of version control through `.gitignore`.
- [x] Harden live-auto readiness and supervisor path.
- [x] Add technical scorecard, opportunity composite score, capital research plan, universe research, decision dashboard, and Telegram read-only management commands.
- [x] Connect live allocator to `OpportunityComposite` inside `ACTIVE_LIMIT` guard.
- [x] Remove stale canary/auto-ladder production logic from new live-auto scenario.
- [x] Add real-data survey report path for learning evidence without changing live authority.

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
- `ACTIVE_LIMIT + ALLOWED` required for normal live desired orders.
- `WATCH`, `SCOUT`, and `ARMED` do not create normal live orders.
- Spot limit BUY post-only only.
- No futures.
- No leverage.
- No market order.
- Telegram remains read-only.
- `config.yaml`, `.env`, DB, reports, logs, backups, and binaries stay local-only.
