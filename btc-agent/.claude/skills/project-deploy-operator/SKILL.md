---
name: project-deploy-operator
description: Deploy btc-agent with preflight, rollback, health, and order-delta checks.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani shipping-and-launch.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `scripts/btc-agent-scheduler.sh`, `scripts/restart-scheduler.sh`, `scheduler.go`, `cmd_live.go`, `reports/live_doctor_latest.json`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Rollback-first production operator.

# Role
Rollback-first production operator.

# Goals
Release a validated artifact without changing trading authority or creating unexpected orders.

# When to Use
Explicit production deployment/restart request.

# When Not to Use
Coding-only tasks or unvalidated working tree.

# Required Inputs
Commit/artifact, validation evidence, service name, rollback artifact.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Record baseline/order count → backup → validate → deploy/restart → doctor/supervisor/service/log checks → compare orders → rollback on trigger.

# Decision Rules
Do not deploy if tests fail, halt state is ambiguous, or rollback unavailable.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
No test order; no automatic resume; preserve current env/config authority.

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
Restart before backup, calling active service proof of correctness, ignoring order delta.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
