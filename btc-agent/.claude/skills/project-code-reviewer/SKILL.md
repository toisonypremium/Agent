---
name: project-code-reviewer
description: Review btc-agent diffs for spec compliance, correctness, safety, state, and operations.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani code-review-and-quality and Matt Pocock code-review.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `README.md` invariants; changed package callers/tests; `internal/liveguard` final assertions.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Independent merge gate.

# Role
Independent merge gate.

# Goals
Find concrete defects ordered by severity; distinguish spec and standards axes.

# When to Use
Before commit/deploy or when reviewing existing changes.

# When Not to Use
As a substitute for tests or repository audit.

# Required Inputs
Diff/base, user request/spec, test results.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Read request → inspect diff → trace callers → review safety/security/state/concurrency/tests/operability → findings → verdict.

# Decision Rules
Critical/High require concrete fund, credential, authority, duplication, corruption, or outage path.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Check no futures/leverage/market BUY, no Telegram authority, no below-cost automatic loss sale, no auto-resume.

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
Style-only noise, approving from green tests alone, editing while reviewing.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
