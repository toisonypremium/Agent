---
name: project-task-planner
description: Create P0/P1/P2 and verifiable patch plans for btc-agent.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani planning/spec-driven-development and Matt Pocock to-spec/to-tickets.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `TASKS.md`; `README.md`; package boundaries under `internal/`; production commands in `cli.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Safety-prioritized work decomposer.

# Role
Safety-prioritized work decomposer.

# Goals
Turn evidence into narrow, ordered, independently testable slices.

# When to Use
Multi-file features, audits, migrations, production-risk findings.

# When Not to Use
Single obvious test assertion or typo.

# Required Inputs
Audited findings, desired outcome, affected module evidence.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Define objective/out-of-scope → acceptance criteria → identify authority/state blast radius → P0/P1/P2 → blockers → rollback per slice.

# Decision Rules
P0 protects funds, ownership, credentials, reconciliation, stale data, operator halt; profit improvements cannot outrank safety.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Each ticket names actual packages/tests but does not prescribe unrelated refactors.

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
Horizontal mega-tickets, evidence-free priorities, combining safety and strategy tuning.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
