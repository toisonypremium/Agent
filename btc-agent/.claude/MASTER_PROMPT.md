# BTC Agent — Master Prompt

## Operating contract

This repository is a safety-first Go 1.22 spot accumulation and DCA automation agent. It analyzes BTC with `internal/agent1`, plans asset accumulation with `internal/agent2`, applies exchange and capital guards in `internal/liveguard`, persists state through `internal/storage`, integrates OKX in `internal/exchange/live`, publishes read-only Telegram views through `internal/notify` and `telegram_commands.go`, and runs production cycles from `scheduler.go` and `cmd_supervisor.go`.

Read this file before work. Select the smallest applicable skill under `.claude/skills/` before changing anything.

## Karpathy rules

1. **Think before coding.** State assumptions and conflicting evidence. Ask when a decision cannot be derived from the repository.
2. **Simplicity first.** Implement the minimum proven change. Do not add speculative abstractions, dependencies, configurability, or adjacent cleanup.
3. **Surgical changes.** Every changed line must trace to the request. Preserve unrelated comments, formatting, runtime reports, and safety code.
4. **Goal-driven execution.** Define observable success criteria before editing. Convert bugs into reproducing tests and features into acceptance checks.
5. Do not broaden scope silently. Report unrelated findings without fixing them.

## Production workflow

1. Audit `git status`, relevant call paths, config, tests, and runtime authority before editing.
2. Name the selected skill(s) and why they apply.
3. Write a patch plan: files, intended behavior, verification, rollback.
4. Change one coherent issue group at a time.
5. Run focused tests after each patch; then use the repository checks relevant to blast radius.
6. Review the diff against both the request and repository safety invariants.
7. Prepare rollback before production actions. Never claim success from compilation alone.

## Repository-specific safety invariants

- Spot only. No futures execution, leverage, shorts, or market BUY orders.
- Normal BUY execution remains limit, post-only, and gated by `ACTIVE_LIMIT`, `ALLOWED`, BTC accumulation confirmation, data health, reconciliation, risk limits, exchange filters, and exposure caps.
- The loss policy in `internal/liveguard/exit_manager.go` and `internal/liveguard/hermes_execution.go` is warning/DCA analysis only below average entry. Never restore automatic stop-loss selling.
- Telegram stays read-only. Never add buy, sell, cancel, close, resume, halt override, or credential authority to `telegram_commands.go`.
- Do not enable live trading or change `BTC_AGENT_ALLOW_AUTO_LIVE`, `execution.real_trading_enabled`, `live.auto_execute`, operator halt, or Hermes authority during coding work.
- Do not place, cancel, replace, or close a real order as validation. Use unit tests, simulator paths, `--dry-run`, saved reports, and read-only doctors.
- Preserve fail-closed behavior in `internal/liveguard`, `cmd_live.go`, `cmd_reconcile.go`, and `cmd_supervisor.go`.
- Do not write secrets into tracked files, prompts, logs, tests, or reports. Protect OKX credentials, `TELEGRAM_TOKEN`, `TELEGRAM_CHAT_ID`, LLM keys, `.env`, `config.yaml`, SQLite databases, and private keys.
- SQLite uses constrained connection behavior. Materialize and close rows before nested queries or writes.
- Public APIs and LLM output provide evidence/context only; they do not gain execution authority.

## Core module boundaries

- Decision: `internal/accumulation`, `internal/agent1`, `internal/agent2`, `internal/flow`, `internal/microstructure`.
- Execution safety: `internal/liveguard`, `hermes_production.go`, `cmd_supervisor.go`, `cmd_live.go`, `cmd_reconcile.go`.
- State: `internal/storage`, `data/`, `reports/`.
- Integrations: `internal/exchange/live`, `internal/exchange`, `internal/notify`, `internal/telegramreport`, `internal/llm`.
- Runtime: `scheduler.go`, `scheduler_lock.go`, `scheduler_heartbeat.go`, `deploy/systemd/btc-agent-immutable.service`.

Do not modify modules outside the task. Explain risk before changing an execution gate, domain state, schema, scheduler cadence, external API behavior, or notification authority.

## Verification ladder

Use the narrowest sufficient checks, then expand by risk:

```bash
gofmt -w <changed-go-files>
go test <affected-packages> -count=1
go test ./... -count=2 -timeout=300s
go vet ./...
go test -race <state-or-runtime-packages> -count=2 -timeout=300s
go build -o bin/btc-agent .
git diff --check
```

Do not run production commands unless explicitly required and safeguarded. Runtime review is read-only by default: service state, doctor, supervisor, open-order count, logs, and reports.

## Required output after every patch

- **Skill đã dùng:**
- **File đã đọc:**
- **File đã sửa:**
- **Nội dung sửa:**
- **Lý do sửa:**
- **Cách test:**
- **Cách rollback:**
- **Rủi ro còn lại:**
- **Bước tiếp theo:**
