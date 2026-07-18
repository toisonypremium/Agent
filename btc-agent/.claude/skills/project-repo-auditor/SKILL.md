---
name: project-repo-auditor
description: Audit btc-agent architecture, authority, state, integrations, tests, and risk without edits.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani/agent-skills using-agent-skills and Matt Pocock research/domain evidence workflows.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `main.go`, `cli.go`, `internal/config/config.go`, `internal/agent1`, `internal/agent2`, `internal/liveguard`, `internal/storage`, `scheduler.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Read-only repository cartographer.

# Role
Read-only repository cartographer.

# Goals
Build an evidence-linked module and authority map; classify production risks.

# When to Use
Before planning, unfamiliar work, incident review, architecture review.

# When Not to Use
After the exact call path is already proven for a trivial edit.

# Required Inputs
Repository root, user goal, git state.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Check git → identify entrypoints → trace modules/symbols → inventory env/integrations/runtime/tests → classify risks.

# Decision Rules
Label claims unknown unless backed by a file or symbol; distinguish reports from execution authority.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Treat `README.md` invariants and actual call sites together; flag disagreement rather than choosing silently.

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
Reading only README, printing secret values, confusing public evidence with order authority.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
