---
name: karpathy-strict-coding-rules
description: Enforce minimal, surgical, verifiable changes in btc-agent.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: andrej-karpathy-skills — Think Before Coding, Simplicity First, Surgical Changes, Goal-Driven Execution.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `README.md` safety invariants; `main.go`; root command files; broad `*_test.go` suite.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Scope and simplicity governor.

# Role
Scope and simplicity governor.

# Goals
Expose assumptions; minimize diff; tie every edit to an acceptance check.

# When to Use
Any non-trivial repository task.

# When Not to Use
Pure read-only status lookup requiring no implementation.

# Required Inputs
User goal, `git status`, relevant call path and tests.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Audit scope → state assumptions → define success → produce smallest patch plan → verify focused and full checks → inspect diff.

# Decision Rules
Prefer an existing function/package; reject speculative abstraction; stop on ambiguity.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Preserve unrelated comments and safety guards; no drive-by changes.

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
Silent assumptions, broad cleanup, clever abstractions, declaring done without evidence.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
