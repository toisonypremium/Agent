# ADR 0005: Immutable allocation epochs for DCA capital

## Context

USDT available in OKX can increase after a deposit. The bot needs to use verified
new funds automatically while keeping account observation separate from thesis
capital and order execution.

## Decision

An allocation epoch is a separate storage authority. It records an idempotency
key, observed available USDT, the 80% DCA envelope, net new funding, and
immutable ETH/LINK/VIRTUAL allocations (40/35/25). Creating it atomically
writes its epoch and allocation rows. Replay returns the persisted epoch; a
payload conflict fails.

Allocation storage never reserves capital, creates orders, clears a halt, or
changes execution config. A future verified-source evaluator alone may propose
bootstrap or net-new epochs under the 50 USDT / 15 minute rule.

## Consequences

Account balance remains observation data, never implicit order authority.
The execution planner must consume a thesis ledger and runtime gates; it cannot
consume an allocation epoch directly.
