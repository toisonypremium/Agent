---
name: trading-code-review
description: Review a BTC Agent branch, commit range, or working-tree diff against repository standards, originating requirements, and trading safety invariants. Use for PR, branch, WIP, or fixed-point code review.
---

# Three-axis trading code review

Read `CONTEXT.md`, relevant ADRs, and `.agents/AGENTS.md`. Pin a fixed point and
verify it resolves. Use merge-base diff for branches and list included commits.

## Review axes

### Standards

Check Go clarity, error handling, context/transaction lifetimes, SQLite
single-connection behavior, deterministic tests, comments, package ownership,
and unnecessary abstraction.

### Spec

Identify the user request, issue, design artifact, or acceptance criteria. Check
that each requirement is implemented and no unrelated behavior changed. If no
spec exists, say so rather than inventing one.

### Safety invariants

Check independently:

- spot/DCA-only constraints;
- no market BUY or automatic stop-loss SELL;
- operator halt and live gates cannot be bypassed;
- thesis provenance is explicit and immutable where required;
- reservations/fills/releases are atomic and idempotent;
- unknown outcomes never release capital;
- rollback leaves no partial writes;
- legacy rows remain compatible without inferred thesis;
- read-only reports do not repair state or grant order authority;
- tests never touch production DB or exchange.

## Findings

Report only actionable findings with file/line evidence and classify:

- `BLOCKER`: could trade unsafely, corrupt capital, bypass authority, or lose
  recovery correctness;
- `HIGH`: material correctness, security, deadlock, or rollback issue;
- `MEDIUM`: maintainability or coverage weakness with realistic impact;
- `LOW`: bounded clarity or cleanup issue.

Distinguish issues introduced by the diff from pre-existing follow-ups. If no
findings exist, state that explicitly and list remaining validation gaps.
