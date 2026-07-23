# Web Console threat model

## Scope

This model covers the future BTC Agent Web Console, its authenticated reverse
proxy, read-model gateway, allowlisted reports and the narrow operator halt
request. The current Phase 0 frontend is fixture-only; it has no API, runtime
or VPS connection.

## Assets

| Asset | Required protection |
|---|---|
| Operator halt state | Integrity, durable audit, no unauthorised clear/resume. |
| Runtime/read-model state | Correct provenance/freshness, confidentiality of operational details. |
| SQLite runtime files | No browser access; scheduler write authority remains unchanged. |
| Credentials/config/environment | Never leave the server or appear in reports/API/logs. |
| Release and evidence provenance | Immutable release/binary hashes and timestamps cannot be silently replaced. |
| Operator identity | Authenticated role, session confidentiality and auditability. |

## Trust boundaries

```text
Browser <-> TLS identity-aware gateway <-> loopback Web Console API
Web Console API <-> typed read model <-> SQLite/report allowlist
Web Console halt request <-> existing validated halt boundary
Scheduler/execution runtime remains outside the Web Console authority boundary
```

## Threats and mitigations

| Threat | Boundary | Mitigation | Verification |
|---|---|---|---|
| Unauthenticated data access | Browser/gateway | OIDC or identity-aware gateway, TLS, default loopback binding, server-side claim validation | unauthenticated/expired/wrong-audience integration tests |
| Role escalation | Gateway/API | Allowlisted roles; never trust public forwarding headers; operator role only for halt requests | viewer/operator/admin matrix tests |
| CSRF/replay halt | Browser/API | SameSite secure session, CSRF token, idempotency key, short session age, append-only receipt | CSRF and duplicate-key tests |
| Halt clear through UI | API/runtime | No resume route, no generic command dispatcher, no CLI passthrough | route inventory and negative regression test |
| Browser execution authority | API/runtime | No order/cancel/config/reconcile endpoints; read DTOs only | static route audit and forbidden-symbol test |
| Secret disclosure | API/files/logs | Allowlists, recursive sanitizer, report catalog, safe errors, no raw config/env | nested redaction and report fixture tests |
| Path traversal/report exfiltration | API/files | Stable catalog IDs, fixed filenames, canonical base directory, content/size caps | traversal and oversized-file tests |
| Stale/misleading status | Read model/UI | timestamps/freshness state per DTO; stale and unavailable UI states; no healthy default | deterministic stale/read failure tests |
| SQLite contention/DoS | Read model/DB | short read deadlines, busy timeout, pagination caps, rate limits; separate process | concurrent read/write fixture test |
| XSS/report injection | Reports/browser | render structured allowlisted data; sanitize Markdown; CSP; no raw HTML | XSS payload UI tests and CSP smoke test |
| Supply-chain compromise | Build | exact version pins, lockfile, npm ci, dependency/security review | lockfile CI and audit review |
| Web restart alters trading runtime | Deployment | separate immutable artifact/service, no service-control privilege | staging restart isolation test |

## Explicitly out of scope

- Real order, cancellation, position mutation, reconciliation mutation and
  configuration changes.
- Clearing/resuming an operator halt.
- Direct public listener, public share links and browser database access.
- Any cloud replication/read-model restoration.

## Residual risks and stop conditions

Do not deploy if identity enforcement, CSP/security headers, report allowlists,
negative authority tests, or runtime-process isolation are incomplete. A
security incident, unexpected endpoint, stale-state misrepresentation or any
sign of execution authority acquisition requires an operator halt and incident
review before proceeding.
