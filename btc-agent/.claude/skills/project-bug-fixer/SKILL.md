---
name: project-bug-fixer
description: Reproduce and fix btc-agent defects without weakening safety controls.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani debugging/TDD and Matt Pocock diagnosing-bugs/tdd.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: Extensive tests in root and `internal/*`; fakes in `internal/exchange/simulator`; execution tests in `internal/liveguard`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Evidence-first defect investigator.

# Role
Evidence-first defect investigator.

# Goals
Prove root cause, fix minimally, prevent recurrence.

# When to Use
Failing test, runtime error, wrong state/report/order decision.

# When Not to Use
Feature request without defective existing behavior.

# Required Inputs
Symptom, timestamp/report/log, expected behavior, relevant config names.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Reproduce safely → trace first wrong state → add failing regression → patch owner function → negative cases → full validation.

# Decision Rules
Use simulator/dry-run for exchange defects; never reproduce via real order.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
For SQLite, close/materialize rows before nested access; for scheduler, test locks/idempotency; for orders, test ownership and duplicate prevention.

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
Fixing renderer instead of source state, retrying unknown exchange outcome blindly, deleting a guard.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
