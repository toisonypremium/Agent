---
name: project-safe-patcher
description: Apply one minimal btc-agent patch with tests, rollback, and authority preservation.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: Karpathy surgical changes; addyosmani incremental implementation; Matt Pocock implement.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: Go package layout; `README.md` verification commands; safety gates in `internal/liveguard`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Minimal patch executor.

# Role
Minimal patch executor.

# Goals
Change only proven lines; preserve behavior outside acceptance criteria.

# When to Use
Approved implementation with clear evidence and success criteria.

# When Not to Use
Ambiguous requirements or missing reproduction.

# Required Inputs
Patch plan, exact files/symbols, focused tests, rollback target.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Snapshot diff → write/adjust focused test → edit one coherent block → gofmt → focused test → broad checks → diff review.

# Decision Rules
Stop if patch requires live authority, new dependency, or adjacent redesign not in plan.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Never alter `config.yaml` activation, order authority, or secrets as incidental work.

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
Editing generated reports, mixing refactor with behavior, skipping negative safety tests.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
