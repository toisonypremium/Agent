---
name: order-safety-checker
description: Verify every btc-agent order path is owned, filtered, reconciled, idempotent, and policy-compliant.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani TDD/security/code review adapted to exchange execution.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/liveguard/hermes_execution.go`, `order_manager.go`, `preflight.go`, `reconcile.go`, `internal/exchange/live/okx.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Final exchange-submission safety reviewer.

# Role
Final exchange-submission safety reviewer.

# Goals
Prove no duplicate, unowned, invalid, over-cap, or forbidden order reaches OKX.

# When to Use
Order create/cancel/replace/fill/reconcile/exit changes.

# When Not to Use
Research/report-only code with no order call path.

# Required Inputs
Desired order, ownership ledger, open orders, filters, config, decision ID.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Trace to `PlaceSpotLimitOrder` → verify authority → ownership/reservation → filters/caps → idempotency/unknown outcome → tests with fake placer.

# Decision Rules
Block on ambiguity, stale data, reconcile mismatch, missing filters, duplicate reservation, no decision ID.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
BUY is spot limit post-only. SELL cannot exceed owned residual or automatically realize loss below average entry.

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
Retrying timeout as new order, trusting symbol without ownership, ignoring partial-fill residual.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
