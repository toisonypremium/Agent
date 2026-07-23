# Current delivery roadmap

**Source fixed point:** `6df4952`
**Runtime profile:** immutable user service, halted-paper.
**Authority:** operator halt remains `ACTIVE`; no canary or real-order approval.

## P0 — Evidence and operational integrity

- [ ] Complete the halted-shadow window ending around `2026-07-30T07:25:43Z`.
  Evidence: continuous observer PASS, exactly one scheduler, fresh lease and
  heartbeat, SQLite integrity, verified backup, zero failed user units.
- [ ] Perform one controlled reboot verification inside the approved shadow
  window. Preserve evidence; do not clear halt.
- [ ] Collect natural paper-order lifecycle evidence under the contract in
  [docs/paper-evidence-contract.md](docs/paper-evidence-contract.md). Do not
  create synthetic orders or mutate production data to satisfy the threshold.
- [ ] Review any persistence/report failure signals. Critical operational state
  must be observable; report-only errors must be visible in runtime evidence.
- [ ] Re-run immutable runtime verifier after every approved deploy/restart.

## P1 — Before any separate canary request

- [ ] Add failure-path coverage for scheduler orchestration, heartbeat,
  ownership, execution guard, exchange parser/timeouts, and stale-data handling.
- [ ] Export and retain CI coverage artifacts; set incremental regression gates
  for safety-critical packages rather than a global vanity threshold.
- [ ] Refactor only narrow orchestration seams in `scheduler.go` and `cmd_live.go`
  behind contract tests. No broad rewrite during the halted-shadow window.
- [ ] Freeze a baseline and evaluate out-of-sample/paper data before any rule or
  sizing change. Manual review is mandatory.

## P2 — Separate explicit canary approval required

A canary is blocked until P0/P1 evidence passes, reconciliation is clean,
backup/restore is verified, operator halt handling is reviewed, and an operator
approves a bounded order cap, monitoring owner, stop conditions, and rollback.
No release, CI result, paper scorecard, or document grants this approval.

## Completed foundations

- [x] Immutable user runtime: SHA-verified release installation, atomic current
  symlink, verified SQLite backup, observer and daily digest.
- [x] Runtime verifier: paper mode, active halt, one scheduler, fresh lease and
  heartbeat, SQLite, backup, and failed-unit checks.
- [x] Spot/DCA safety controls: no futures, leverage, shorts, market BUY, or
  automatic stop-loss SELL; unknown outcomes retain capital reservation.
- [x] CI: formatting, vet, static analysis, vulnerability check, Linux race
  tests/build, config check, secret scan, immutable drills.
- [x] Evidence-driven AI delivery workflow and task contract.

## Standard verification

```bash
make verify
make linux-build
```

Linux CI remains authoritative for `go test -race`. Do not use production DB,
credentials, or exchange calls for development validation.
