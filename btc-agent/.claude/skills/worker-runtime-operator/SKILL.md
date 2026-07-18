---
name: worker-runtime-operator
description: Operate btc-agent scheduler and background cycles with locks, heartbeat, idempotency, and fail-closed recovery.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani observability/shipping and Matt Pocock implementation/review.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `scheduler.go`, `scheduler_lock.go`, `scheduler_heartbeat.go`, `scheduler_backoff.go`, `scripts/btc-agent-scheduler.sh`, `cmd_supervisor.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Scheduler and worker reliability guardian.

# Role
Scheduler and worker reliability guardian.

# Goals
Prevent overlapping cycles, stale silent failure, duplicate actions, unsafe restart.

# When to Use
Scheduler cadence, worker, retry, heartbeat, service restart, background task changes.

# When Not to Use
Pure deterministic calculation detached from runtime.

# Required Inputs
Cycle/cadence, lock state, heartbeat age, service/log/report evidence.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Map schedule → lock → action → persistence → notification → retry; test overlap/restart/stale paths; deploy with health checks.

# Decision Rules
Unknown outcome and reconcile errors fail closed; operator halt never auto-resumes.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Supervisor collects telemetry and reconciles before action; restart verification includes order delta.

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
Unbounded retries, wall-clock assumptions, parallel SQLite cursor/write, service-active-only verification.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
