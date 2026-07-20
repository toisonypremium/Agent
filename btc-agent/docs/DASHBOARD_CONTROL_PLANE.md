# HERMES Control Room v3

## Authority planes

The dashboard keeps market authority, operator capability, Hermes mode and Circuit research authority separate.

- Market: `ALLOWED | BLOCKED | HALTED | UNKNOWN`, derived by deterministic Go gates.
- Operator: capability whitelist evaluated from verified identity, feature flags and fresh safety evidence.
- Hermes: UI policy controls stop at `canary`; `autonomous` is absent from the web contract.
- Circuit: always `RESEARCH_ONLY` with `execution_intent=null`.

There is no web `PLACE_ORDER`, `BUY`, `SELL`, `CANCEL_ORDER` or Circuit execution capability.

## Feature flags

`BTC_AGENT_WEB_HALT_ENABLED=true` registers only authenticated session and HALT endpoints. Default is disabled. Phase 3 controls are contracts and storage only; they are not registered in production until separate per-capability activation gates pass.

## HALT security

- Cloudflare Access JWT is the identity authority.
- Authenticated email header must equal the verified JWT identity.
- CSRF token is random, session-bound and expires after one hour.
- Sensitive requests require an idempotency key and reject replay.
- HALT requires typed `HALT` confirmation and an 8–500 character reason.
- The request is appended to runtime audit before halt state changes.

## Change workflow

Risk increases require two confirmations by identities distinct from the requester. Changes bind to a safety snapshot ID/hash, expire within 30 minutes and reject unknown actions. Decreases require validated before/after values and cannot silently increase another cap.

## Rollback

Disable mutation controls by removing the feature flag and restart only the web service. Read-only dashboard v3 remains available. Static/API rollback does not alter scheduler, trading config, Circuit timer, production DB or evidence archives.
