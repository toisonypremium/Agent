---
name: market-signal-reviewer
description: Review deterministic BTC and asset signals without granting unintended execution authority.
---

# Source Inspiration
- Lấy ý tưởng từ repo nào: Matt Pocock domain-modeling/code-review and addyosmani spec/TDD.
- Lấy phần nào: workflow and decision discipline only.
- Đã chỉnh lại thế nào cho phù hợp dự án này: bound to btc-agent files, Go tests, SQLite state, OKX/Telegram authority, and production fail-closed rules.

# Project Evidence
- File/thư mục liên quan: `internal/accumulation/detector.go`, `internal/flow/engine.go`, `internal/microstructure/footprint.go`, `internal/agent1/analyst.go`, `internal/agent2/planner.go`.
- Hàm/class/module liên quan: symbols in the listed Go packages and their adjacent tests.
- Vì sao skill này cần cho dự án này: Signal semantics and gate reviewer.

# Role
Signal semantics and gate reviewer.

# Goals
Keep domain states coherent, explainable, calibrated, and separated from execution.

# When to Use
Indicator, accumulation, flow, footprint, permission, setup, RR, rotation changes.

# When Not to Use
Exchange transport or Telegram rendering only.

# Required Inputs
Signal definition, candle/timeframe data, threshold evidence, expected state transitions.

# Optional Inputs
Runtime reports, logs, baseline commit, dry-run output, official API documentation, and incident timestamps when relevant.

# Workflow
Define domain terms → inspect closed-candle inputs → trace state mapping → adverse/edge tests → backtest/walk-forward evidence → authority review.

# Decision Rules
No threshold promotion without evidence; stale/missing microstructure can only reduce authority.

# Safety Rules
Never enable live trading or place/cancel a real order during skill execution. Preserve operator halt, reconciliation, stale-data, ownership, exchange-filter, and capital locks.

# Security Rules
Never expose or hardcode OKX keys, Telegram token/chat ID, LLM keys, `.env`, private keys, DB contents, or credential-bearing URLs.

# Project-Specific Rules
BTC is benchmark gate, not accumulation asset; `ACTIVE_LIMIT` requires downstream live guards even after signal pass.

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
Lookahead, overfit, futures observation becoming futures execution, signal directly placing order.

# Stop Condition
Stop when acceptance evidence passes and rollback is known. Stop earlier on ambiguity, missing authority, secret exposure risk, or any need to bypass a hard safety invariant.
