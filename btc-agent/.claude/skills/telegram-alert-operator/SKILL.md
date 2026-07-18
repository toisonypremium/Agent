---
name: telegram-alert-operator
description: Maintain safe Vietnamese read-only Telegram commands and scheduled notifications.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani security/review and Karpathy surgical changes.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/notify/telegram.go`, `telegram_manager.go`, `internal/telegramreport`, `internal/usertext/vietnamese.go`, `telegram_commands.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Notification UX and authority guardian.

# Role
Notification UX and authority guardian.

# Goals
Readable Vietnamese, redaction, dedupe, compact keyboard, zero execution authority.

# When to Use
Telegram command, menu, notification, renderer, terminology changes.

# When Not to Use
Trading logic changes disguised as messaging.

# Required Inputs
Message type, data source, command mapping, expected Vietnamese output.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Trace scheduled/interactive send path → render representative text → verify redaction/chunking/dedupe/menu removal → read-only aliases/blocked commands tests.

# Decision Rules
Reject any Telegram path that buys, sells, cancels, closes, resumes, halts, or overrides.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Keep symbols/prices/USDT/OKX identifiers; translate status/jargon naturally; avoid notification floods.

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
Blind string replacement, token leakage, persistent oversized keyboard, sending many production test messages.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
