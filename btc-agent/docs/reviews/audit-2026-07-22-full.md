# Full Codebase Audit — 2026-07-22

## Summary

| Metric | Value |
|---|---|
| Root .go files | 57 |
| Root source lines | ~11,000 |
| Internal packages | 33 |
| Internal .go files | ~239 |
| Total test packages | 31 |
| go vet errors | 0 |
| Build errors | 0 |

## Critical Issues

### 1. Duplicate utility functions (5 definitions of `uniqueStrings`, 8 of `firstNonEmpty`)
Each package copied the same 5-line helper. The root has `uniqueStringsMain`
and `firstNonEmpty` in `cmd_status.go` — package-level orphans.

**Fix:** Create `internal/utils/strings.go` with canonical versions. Make each
package import from there; remove local copies.

### 2. `cmd_live.go` is 1,112 lines — too many responsibilities
Contains: proof runner, readiness, live doctor, auto-live orchestration,
markdown formatting, reporting helpers.

**Fix:** Extract markdown/reporting helpers to `cmd_live_reports.go`. Extract
doctor logic to `cmd_live_doctor.go`. Cap `cmd_live.go` at ~400 lines.

### 3. `scheduler.go` is 670 lines — one mega-function of 603 lines
`runScheduler` is a monolith that schedules everything. Hard to test, hard
to modify safely.

**Fix:** Extract each scheduled task to its own `func schedulerXxx(...)`.
`runScheduler` becomes a coordinator only.

### 4. `telegram_commands.go` is 669 lines — mixed read-only and mutation paths
`buildReadOnlyTelegramCommandReply` alone is 103 lines.

**Fix:** Split into `telegram_commands_query.go` and `telegram_commands_admin.go`.

### 5. `saveJSONFile` lives in `cmd_maintenance.go` but is called 24 times across the project
It belongs in `internal/reportio`.

**Fix:** Move `saveJSONFile` to `internal/reportio`. Update 24 callers.

### 6. `config.Validate()` has a single function of 353+ lines
Config validation is a monolith. No section separation.

**Fix:** Split into sub-validators: `validateLiveConfig`, `validateRiskConfig`,
`validatePortfolioConfig`, `validateDataConfig`. Top-level `Validate` composes them.

### 7. Dead entry points — `autoLiveOrderMarkdown` and `liveOrderAttemptText`
Both defined in `cmd_live.go` but never called in current flow (managed path
replaced manual proof). Adds ~60 lines of confusion.

**Fix:** Remove both.

### 8. Untracked new files mixed with modified tracked files in git
Pre-existing dirty worktree has many modified tracked and untracked source files,
making it impossible to see what this branch specifically added.

**Fix:** Stage only intentional adds, review diff carefully, commit to current branch.

## Package Dependency Map (root → internal)

Core (required for live-auto):
- `config` (37 root uses) — configuration
- `storage` (31 root uses) — SQLite DB
- `agent2` (27) — market planner/decision
- `liveguard` (21) — managed order execution
- `agent1` (17) — market analysis
- `market` (10) — raw market data fetch/parse

Supporting runtime:
- `reportio` (13) — JSON/markdown file output
- `exchange/live` (7) — OKX client
- `opsplan` (6) — ops plan generation
- `telegramreport` (5) — Telegram message formatting
- `microstructure` (4) — order book / microstructure
- `flow` (4) — BTC flow analysis
- `hermesoperator` (4) — Hermes operator

Optional/research:
- `hermesagent` (3), `llm` (4), `research` (2), `aiagent` (1), `aieval` (1)
- `backtest` (2), `learning` (1), `circuitresearch` (1)

No callers found: `utils` (empty package, should be removed or populated)

## Refactor Plan (ordered by impact)

1. Create `internal/utils/strings.go` with `FirstNonEmpty`, `UniqueStrings`.
   Update all 8+5 copy-paste definitions across packages.

2. Move `saveJSONFile` to `internal/reportio`. Fix 24 callers.

3. Remove `autoLiveOrderMarkdown` and `liveOrderAttemptText` from `cmd_live.go`.

4. Split `cmd_live.go` into `cmd_live.go` (orchestration), `cmd_live_reports.go`
   (formatting helpers), `cmd_live_doctor.go` (doctor/readiness).

5. Split `config.Validate()` into sub-validators.

6. Refactor `runScheduler` into composed task functions.

7. Split `telegram_commands.go`.

8. Stage and commit intentional changes.

## Safety constraints (non-negotiable)

- `go test ./...` must pass after every step.
- `go vet ./...` must stay clean.
- `go build .` must succeed.
- `git diff --check` must pass.
- No config, DB, scheduler, or credentials modified.
- No exchange calls, order placement, or live mode enabled.
- fail-closed gating in `requireAutoLiveRuntime` and `AssertManagedExecutionAllowed`
  must not weaken.
