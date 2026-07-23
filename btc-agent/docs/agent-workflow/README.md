# AI-assisted delivery workflow

AI is an implementation and review assistant, never a source of trading or
runtime authority. This workflow stays small, observable, and risk-based.

## Principles

- **One task, one contract.** Define outcome, non-goals, authority boundaries,
  acceptance evidence, and approval needs before a multi-file change.
- **Workflow before autonomy.** Use deterministic steps for known work; use
  open-ended exploration only for bounded research or refactors.
- **Ground truth wins.** Tests, diffs, tool output, CI, and runtime verification
  override an agent narrative.
- **Small reversible increments.** One concern per commit; each commit can be
  independently tested or reverted.
- **Human approval at authority boundaries.** Deploy, restart, credentials,
  configuration, halt, and execution changes require explicit approval.
- **Safety is not delegated.** Reports, LLMs, plans, and evaluations cannot
  acquire execution authority.

## Delivery loop

### 1. Route the request

| Change class | Required path |
|---|---|
| Documentation or isolated cleanup | Contract plus focused checks |
| Feature or bug fix | Contract, deterministic test, `make verify` |
| Storage, execution, reconcile, capital, lifecycle, ownership | Contract, design review, safety matrix, full gate |
| Deploy, restart, config, credentials, authority | Full relevant gate plus immediate explicit approval |
| Research | Written findings; no production mutation |

Ambiguous outcome or approval boundary means stop and clarify.

### 2. Create a task contract

Use [task-contract-template.md](task-contract-template.md) in an issue, PR, or
work record. Required fields: observable outcome, non-goals, affected authority,
safety constraints, acceptance commands, approval boundary, and rollback/stop
condition.

### 3. Design and implement

1. Read `CONTEXT.md`, `.agents/AGENTS.md`, and the relevant skill.
2. Compare two designs when authority or transaction ownership changes; record
   durable decisions in an ADR.
3. Add the smallest deterministic test at a public seam.
4. Keep commits focused. Parallelize only independent research or review.
5. Treat unexpected tool output as a blocker, never as permission to improvise.

### 4. Verify independently

```bash
make verify
```

For capital/order/reconcile work, also cover applicable cases: success,
idempotent replay, collision, rollback, legacy compatibility, unknown-outcome
fail-closed, provenance conflict, and restart/readback. Linux CI is the
race/static-analysis/vulnerability authority; local Termux may not support
`-race`.

### 5. Review and complete

Review separately:

- **Spec:** task contract met; non-goals untouched.
- **Safety:** no bypass of halt, execution guards, provenance, reservations, or
  unknown-outcome handling.
- **Quality:** deterministic tests, current docs, minimal diff.
- **Operations:** release SHA, backup, rollback, and immutable verifier evidence.

Use [AGENT_DONE_CHECK.md](../../AGENT_DONE_CHECK.md) before reporting done.

## Authority ladder

```text
source change
  -> local verification
  -> CI verification
  -> explicit deployment approval
  -> immutable release install
  -> read-only runtime verification
  -> time-bound halted-shadow evidence
  -> separate canary approval
```

No level grants the next one. A green build does not authorize deploy; deploy
does not authorize real execution; a release never clears operator halt.

## References

- [Anthropic: Building Effective AI Agents](https://www.anthropic.com/research/building-effective-agents)
- [Project safety context](../../CONTEXT.md)
- [Project rules](../../.agents/AGENTS.md)
- [Trading code review](../../.agents/skills/trading-code-review/SKILL.md)
