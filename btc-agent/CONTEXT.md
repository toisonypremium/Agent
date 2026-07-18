# BTC Agent domain context

BTC Agent is a spot-only, DCA-oriented trading and operations system. Planning,
capital reservation, exchange execution, reconciliation, lifecycle projection,
and reporting are separate authorities.

## Ubiquitous language

### Thesis

Explicit investment provenance identified by `thesis_id`. A thesis owns one
immutable symbol and its capital/lifecycle records. Identity is never inferred
from symbol.

### Investable envelope

Caller-supplied portfolio planning limit. It is compared by a read-only audit;
it is not storage reservation authority.

### Max exposure

Per-thesis capital ceiling persisted in the thesis capital ledger.

### Reservation

Capital committed atomically when a thesis-aware BUY order is recorded. It
moves available DCA capital to reserved capital before exchange submission.

### BUY_FILL

Idempotent capital journal event for a confirmed reconciled BUY fill. It moves
capital from reserved to filled inside the same transaction as fill, position,
and lifecycle projection.

### RELEASE

Idempotent journal event returning unused reservation after a known terminal
outcome. Unknown exchange outcomes never release capital.

### Projection drift

Difference between persisted ledger values and values reconstructed from the
journal and active thesis BUY orders. Audits report drift but never repair it.

### Reconciled fill

Confirmed exchange fill applied through the atomic storage boundary. It updates
position event, fill snapshot, position, thesis capital, residual release, and
lifecycle in one transaction.

### Terminal outcome

Known final order status such as CANCELLED, REJECTED, or final FILLED. It may
release unused reservation according to persisted fills.

### Unknown outcome

Exchange state that cannot be proven terminal. It requires manual reconciliation
and keeps reservation locked.

### Position lifecycle

Thesis state projected from confirmed fills and explicit review transitions.
Lifecycle records position context but do not create exchange authority.

### DCA authority

Permission to increase spot exposure. It requires planner/risk permission,
lifecycle allowance, and successful atomic capital reservation.

### SELL authority

Permission to submit a SELL. Loss, invalidation, evaluator output, or lifecycle
state alone never grants automatic SELL authority.

### Operator halt

Manual safety lock that blocks execution. It never resumes automatically and
cannot be bypassed by Hermes, planning, or reconciliation.

## Core safety model

- Spot-only; no futures, leverage, or shorting.
- DCA BUY uses limit/post-only execution; no market BUY.
- No automatic stop-loss SELL.
- Unknown exchange outcomes fail closed.
- Read-only audit/evaluation cannot mutate state or place orders.
- Development tests use temporary databases and no live credentials.
