---
name: trading-codebase-design
description: Design or improve deep Go modules and clean seams in BTC Agent while preserving trading authority and recovery invariants. Use for interface design, module boundaries, testability, architecture, or refactoring proposals.
---

# Deep modules for BTC Agent

Read `CONTEXT.md`, ADRs, and `.agents/AGENTS.md` first.

A deep module exposes a small interface that owns substantial correct behavior.
A seam is where behavior can vary without changing callers. Keep authority and
transaction ownership local rather than distributing invariants across call
sites.

## Design process

1. State the caller problem and safety invariants.
2. Identify the current authority owner and transaction boundary.
3. Design at least two interfaces; compare misuse resistance, rollback,
   idempotency, testability, migration, and legacy behavior.
4. Prefer the narrowest interface that makes the safe path easy and unsafe
   composition difficult.
5. Avoid abstraction with one caller unless it reduces authority surface or
   creates a meaningful test seam.
6. Record durable choices as ADRs before broad refactors.

Existing deep-module examples:

```text
ApplyReconciledLiveFill
ReserveManagedLiveOrderWithThesis
SaveTerminalLiveOrderStatusAndRelease
PortfolioCapitalInvariantAudit
EvaluateThesisPositionLifecycle
```

Do not merge planning, execution, reconciliation, lifecycle, and reporting into
one interface. Read-only modules may propose/block/report but cannot acquire
order authority.

See `DEEPENING.md` and `DESIGN-IT-TWICE.md` for review prompts.
