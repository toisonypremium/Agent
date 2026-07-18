---
name: state-recovery-guardian
description: Protect SQLite ledgers, runtime state, reports, and restart recovery from corruption or duplication.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani security/TDD and Matt Pocock domain modeling.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/storage/sqlite.go`, storage tests, `cmd_reconcile.go`, `scheduler_lock.go`, `reports/exit_peak_tracker.json`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: State source-of-truth and recovery reviewer.

# Role
State source-of-truth and recovery reviewer.

# Goals
Keep ownership/order/runtime state durable, idempotent, backed up, and fail closed.

# When to Use
Schema, query, ledger, markout, runtime-state, migration, restart/recovery work.

# When Not to Use
Stateless renderer change.

# Required Inputs
DB path/schema, transaction/query flow, state ownership, recovery scenario.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Identify source of truth → backup → inspect cursor/transaction scope → define idempotency → simulate interruption/restart → reconcile → verify no duplicates.

# Decision Rules
Never infer ownership from balance alone; materialize/close rows before nested DB access; block on mismatch.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Do not edit production DB during code tests; reports are projections unless explicitly the persisted source.

# Output Format
- Skill used and objective
- Evidence read (files and symbols)
- Assumptions and scope
- Findings or patch plan
- Verification commands/results
- Rollback
- Residual risks and stop reason

# Quality Checklist
- [ ] Every claim/change traces to project evidence.
- [ ] Scope is minimal and no dependency was added.
- [ ] Relevant negative safety behavior is tested.
- [ ] Secrets and live authority were not exposed or changed.
- [ ] Diff/outputs were reviewed against the user goal.

# Common Mistakes to Avoid
Replacing DB wholesale, writes under open cursor, destructive migration without rollback, auto-resume after failure.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
