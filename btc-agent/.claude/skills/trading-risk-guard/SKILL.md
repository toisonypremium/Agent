---
name: trading-risk-guard
description: Protect spot-only DCA capital and hard safety invariants during btc-agent changes.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani security/review plus Matt Pocock domain-modeling, adapted to trading domain.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/agent1/analyst.go`, `internal/agent2/planner.go`, `internal/liveguard/risk_governor.go`, `exit_manager.go`, `config.yaml.example`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Trading policy invariant gate.

# Role
Trading policy invariant gate.

# Goals
Prevent authority escalation, unsafe sizing, and loss-policy regressions.

# When to Use
Any signal, sizing, DCA, exit, portfolio, risk, or strategy change.

# When Not to Use
Notification-only wording with no decision data change.

# Required Inputs
Policy request, affected decision path, config schema, capital invariants.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Map market→plan→guard→executor → enumerate allowed/blocked states → test caps and adverse cases → review runtime authority.

# Decision Rules
Weak market may size/wait but must not bypass hard locks; loss below cost warns/DCA-review only.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Spot only; percentage capital allocation; no futures, leverage, short, market BUY, automatic stop-loss below average entry.

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
Optimizing profit before evidence, fixed asset quantities, conflating research with execution.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
