# Skill Sources and Project Mapping Report

## 1. Current repository audit

### A. Project identity

`btc-agent` is a safety-first autonomous spot accumulation/DCA trading agent. Evidence: `README.md`, `main.go`, `cli.go`, `internal/agent1`, `internal/agent2`, `internal/liveguard`, and `hermes_production.go`.

### B. Tech stack

- Go 1.22 module `btc-agent`: `go.mod`.
- YAML configuration: `gopkg.in/yaml.v3`, `internal/config/config.go`.
- Embedded SQLite: `modernc.org/sqlite`, `internal/storage/sqlite.go`.
- Shell/Python operational scripts: `scripts/`.
- Standard-library HTTP integrations for OKX, Binance/public data, Telegram, RSS, and LLM-compatible APIs.

### C. Entrypoint

- `main.go:main` calls the CLI.
- `cli.go` dispatches market, research, live doctor/audit/reconcile/supervisor, scheduler, Telegram, maintenance, and control-plane commands.
- Build/run evidence: `Makefile`, README command sections, `go build -o bin/btc-agent .`.

### D. Core modules

| Module | Evidence | Important symbols / role |
|---|---|---|
| BTC accumulation | `internal/accumulation/detector.go` | `Analyze`, `AnalyzeWithFootprint`; deterministic market phase |
| BTC gate | `internal/agent1/analyst.go` | `MarketAnalysis`, `Analyze`; benchmark regime and permission |
| Asset planner | `internal/agent2/planner.go` | `Plan`, `BuildPlan`, `BuildPlanWithBenchmarks`; candidate/layer authority |
| Flow/microstructure | `internal/flow/engine.go`, `internal/microstructure/footprint.go` | multi-frame flow and footprint evidence |
| Execution safety | `internal/liveguard/` | preflight, risk governor, reconciliation, ownership, managed orders, exits |
| Hermes production | `hermes_production.go`, `cmd_supervisor.go` | guarded actions and supervisor cycle |
| Exchange | `internal/exchange/live/okx.go` | `NewOKXFromEnv`, signed spot API |
| State | `internal/storage/sqlite.go` and storage tests | SQLite order/ledger/runtime/evidence source |
| Telegram | `internal/notify/telegram.go`, `telegram_commands.go`, `internal/telegramreport` | read-only commands and notifications |
| Runtime | `scheduler.go`, lock/heartbeat/backoff files, `deploy/systemd/btc-agent-immutable.service` | periodic production orchestration |

### E. Config, env, secrets

- Config schema: `internal/config/config.go`; examples: `config.yaml.example`, `btc-agent.env.example`.
- Sensitive env names are consumed by `internal/exchange/live/okx.go`, `internal/llm/client.go`, `cmd_notify.go`, and `telegram_commands.go`.
- Protect OKX key/secret/passphrase, Telegram token/chat ID, LLM key, `.env`, real `config.yaml`, DB, reports/logs/backups, and private keys. Values were not copied into these files.

### F. Integrations

- OKX private spot execution and account reads.
- Binance/public microstructure and market data.
- Telegram notifications/read-only commands.
- SQLite persistence.
- RSS/free APIs and LLM-compatible analysis as context only.
- Scheduler/supervisor/background maintenance.

### G. Runtime/deploy

No project Docker definition was found. Production evidence is shell scheduler scripts plus a user-level systemd service on the VPS. Logging/report projections are under `logs/` and `reports/`; heartbeat and lock behavior are implemented in Go.

### H. Tests

The repository has extensive root and package `*_test.go` coverage, including agent states, backtests, OKX parsing/fill history, liveguard execution/reconciliation/ownership, SQLite, Telegram, scheduler, and restart behavior. Gaps requiring continued attention: no real-exchange test should be added; production service/timer definitions are outside the repository; external-provider failure matrices and cross-command recovery remain high-value review areas.

### I. Production risks

- **Critical:** execution authority regression; duplicate/unowned order; leaked exchange credentials; reconcile mismatch ignored; automatic loss sale below average entry restored.
- **High:** stale data used as permission; SQLite recovery/deadlock/corruption; unknown exchange outcome retried; operator halt auto-resumed; Telegram/LLM gains control authority.
- **Medium:** scheduler overlap or stale heartbeat; notification flooding/mixed terminology; public API degradation interpreted as valid zero; report/source-of-truth drift.
- **Low:** documentation drift, stale test wording, non-critical report formatting.

Git was present. The working tree had unrelated untracked sibling directories (`../backups/`, `../quarantine-old-artifacts-*`, `../rollbacks/`, `../sync-inbox-*`). They were reported and not touched. No commit was created.

## 2. What was adapted from andrej-karpathy-skills

Adapted, not copied: explicit assumptions, simplicity before abstraction, surgical scope, goal/acceptance checks, and verification loops. These became the master rules and the `karpathy-strict-coding-rules`/safe patching discipline.

## 3. What was adapted from addyosmani/agent-skills

Selected: skill selection, spec/planning gates, TDD, debugging, multi-axis review, security threat modeling, incremental implementation, observability/runtime verification, and rollback-first shipping. Adapted to Go packages, SQLite constraints, exchange unknown outcomes, Telegram read-only authority, and production doctor/supervisor evidence.

## 4. What was adapted from mattpocock/skills

Selected: facts-from-repo before user questions, spec synthesis, small dependency-aware tickets, implement-from-evidence, behavior-oriented TDD, standards+spec review, and domain language for market/order/state transitions. The interview workflow is represented as a stop/clarify rule rather than a generic chat skill.

## 5. Rejected source material

- Frontend/UI/browser/Core Web Vitals: no web frontend evidence.
- TypeScript-specific architecture/deep-module skills: project is Go.
- Personal writing, Obsidian, article editing, exercises, plugin setup: unrelated.
- Generic CI/CD setup: no request and no repository CI evidence requiring a dedicated skill.
- Performance optimization as a standalone skill: no measured performance requirement; measure-first review can be invoked within runtime/state work.
- Prototype and interface-design workflows: no demonstrated need for a separate skill.
- External issue-tracker publishing: no configured tracker evidence.
- Deprecated/in-progress source skills: avoided unless their stable underlying clarification principle was independently supported.

## 6. Source → project → created skill mapping

| Source | Original idea | Apply? | Project evidence / reason | Created skill |
|---|---|---:|---|---|
| Karpathy | Think, simplify, surgical, goal-driven | Yes | Safety-sensitive broad Go repo | `karpathy-strict-coding-rules` |
| Addy | using-agent-skills | Yes | Multiple distinct execution/state/integration workflows | `project-repo-auditor`, master selection rule |
| Addy | spec/planning | Yes | `TASKS.md`, many coupled packages | `project-task-planner` |
| Addy + Matt | incremental implement | Yes | Package/test seams | `project-safe-patcher` |
| Addy + Matt | debugging/TDD | Yes | extensive tests/simulators | `project-bug-fixer`, `project-test-checker` |
| Addy + Matt | code review | Yes | production authority invariants | `project-code-reviewer` |
| Addy | security | Yes | OKX, Telegram, env, LLM, SQLite | `project-security-reviewer` |
| Addy | shipping/observability | Yes | scheduler scripts, doctor/supervisor/reports | `project-deploy-operator`, `worker-runtime-operator` |
| Matt | domain modeling | Yes | market phases, plan states, order intents | `market-signal-reviewer`, supports trading/order skills |
| Adapted safety workflows | trading risk | Yes | agent1/agent2/liveguard | `trading-risk-guard` |
| Adapted TDD/security | exchange orders | Yes | `hermes_execution.go`, reconciliation | `order-safety-checker` |
| Adapted security/review | Telegram | Yes | notify/report/commands paths | `telegram-alert-operator` |
| Adapted TDD/domain | persistence/recovery | Yes | SQLite ledger/runtime state | `state-recovery-guardian` |
| Addy source-driven/security | external resilience | Yes | OKX/Binance/Telegram/RSS/LLM clients | `external-api-resilience` |
| Addy performance | standalone optimization | No | no measured requirement | none |
| Matt grill-with-docs | clarification | Partial | useful only when a real decision remains | stop/clarify rules; no generic skill |
| Matt to-tickets | external tracker publishing | No | no tracker configured | planning output only |
| UI/TypeScript/personal skills | unrelated workflows | No | no project evidence | none |

## 7. Risk of misusing external skills

Generic skills can accidentally widen scope, add dependencies, prefer web/TypeScript assumptions, turn research into execution authority, over-test through real APIs, or deploy without understanding operator halt and order state. The adapted skills therefore require local evidence, prohibit live validation, preserve fail-closed controls, and stop if authority is ambiguous.

## 8. Generated skill inventory

Sixteen skills were generated. Every skill has explicit Project Evidence and maps to a real repository package or runtime path. No generic performance/UI/CI/personal skill was generated.
