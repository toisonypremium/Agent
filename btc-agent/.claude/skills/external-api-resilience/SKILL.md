---
name: external-api-resilience
description: Harden OKX, Binance/public, Telegram, RSS, and LLM integrations without authority leakage.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani security, source-driven development, observability and error recovery.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/exchange/live/okx.go`, `internal/exchange/binance_microstructure.go`, `internal/notify/telegram.go`, `internal/research/rss.go`, `internal/llm/client.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: External boundary resilience reviewer.

# Role
External boundary resilience reviewer.

# Goals
Bound timeouts/retries, validate responses, redact secrets, distinguish unknown outcomes, degrade safely.

# When to Use
Any HTTP provider, exchange, Telegram, research feed, or LLM client change.

# When Not to Use
Internal pure function with no external data.

# Required Inputs
Official API contract, request/response, timeout/retry policy, authority and freshness expectations.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Verify source docs → classify read/write/unknown outcome → validate/time-bound → retry only idempotent operations → redact/log health → failure tests.

# Decision Rules
Public/LLM failure reduces evidence, never grants execution. Exchange write timeout requires reconciliation before retry.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Futures data is observation only. Telegram errors must not leak token-bearing URLs. Stale data blocks or demotes authority.

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
Blind retries, default HTTP client without bounds, treating malformed response as zero, logging credentials.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
