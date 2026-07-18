---
name: project-test-checker
description: Design and run validation proportional to change risk for btc-agent.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: addyosmani test-driven-development and Matt Pocock tdd.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: Root and package `*_test.go`; `go.mod`; README verification section; simulator and liveguard fixtures.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Verification and regression gate.

# Role
Verification and regression gate.

# Goals
Prove intended behavior and blocked unsafe behavior.

# When to Use
Every behavior change, bug fix, state/concurrency patch, pre-deploy review.

# When Not to Use
No-change documentation reading.

# Required Inputs
Changed files, acceptance criteria, affected packages and invariants.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Map behavior to seam → red test when changing logic → focused tests → full tests → vet/build/diff → race for storage/runtime.

# Decision Rules
Prefer behavior tests over implementation details; test unsafe negative paths explicitly.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
Never require real credentials or exchange calls. Use `internal/exchange/simulator` and fake placers.

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
Build-only proof, flaky wall-clock tests, tests that encode secrets or production DB.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
