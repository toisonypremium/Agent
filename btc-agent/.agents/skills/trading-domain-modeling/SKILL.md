---
name: trading-domain-modeling
description: Build and sharpen BTC Agent domain terminology, bounded authority, lifecycle rules, CONTEXT.md, and architectural decisions. Use when defining or changing thesis, capital, order, fill, reconciliation, lifecycle, or operator concepts.
---

# Trading domain modeling

Read `.agents/AGENTS.md` first. Treat code and tested behavior as evidence; do
not invent trading authority while documenting it.

## Workflow

1. Collect ambiguous or overloaded terms from requirements, code, tests, and
   operational reports.
2. Define each term by invariant, owner, lifecycle, and what it explicitly does
   not authorize.
3. Challenge definitions with replay, crash, restart, partial fill, unknown
   outcome, legacy row, operator halt, and market-loss scenarios.
4. Update `CONTEXT.md` only when terminology is resolved.
5. Create `docs/adr/NNNN-title.md` lazily for durable decisions with competing
   alternatives and consequences.

## Required authority distinctions

Keep planning, reservation, exchange execution, reconciliation, lifecycle, and
manual review separate. A projection/evaluator/report may observe or block; it
must not silently become order authority. A confirmed SELL fill may close a
lifecycle, but lifecycle state cannot initiate that SELL.

Use the templates in this skill directory for new context sections and ADRs.
