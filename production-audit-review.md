# Agent V2 audit/review — 2026-07-20

## Executive verdict

**Conditional pass for halted/canary-safe operation. No approval for unrestricted
live trading.** V2 is running as one fenced owner and current gates prevented orders.

## Findings

### P0 — Cloud publisher is not wired to the runtime (open)

Static audit found adapters and mock tests, but no production composition of
`supabase.Publisher`, `r2.Publisher`, or `outbox.Worker`; `EnqueueOutbox` is only the
SQLite adapter. Cloud sync is therefore not evidenced in the running service.
Provision real endpoints/credentials and wire publishers through the composition
root before relying on Supabase/R2 durability.

### P1 — Lease expiry observation (open monitoring concern)

At the audit command time, the database showed fencing token `2` with expiry
`1784537819`, while the observation timestamp was `1784537983`. Doctor/process were
healthy, but the stored lease was stale at that instant. Investigate heartbeat/renewal
cadence and ensure scheduler lease renewal is continuous; fail closed if renewal is
not fresh. Do not treat process liveness as ownership validity.

### P1 — Healthcheck/systemd ownership is not fully managed (open operational debt)

The VPS healthcheck timer remains active. The user-level wrapper was replaced with V2,
which avoids restarting V1, but root-owned timer/service management could not be
changed because sudo required an interactive password. Verify the timer invokes only
V2 and add a proper systemd unit with explicit `User`, `EnvironmentFile`, lock and
restart policy.

### P1 — R2 publisher contract is incomplete (open)

The adapter accepts a pre-signed URL but does not implement production signing or a
credential provider. It is suitable only when an external presign service exists.

### P2 — Supabase idempotency model is too generic (open)

Publisher uses `on_conflict=id` for every event. Several read-model tables use
idempotency keys/client order IDs rather than a universal `id` payload contract. Map
event type to table and conflict columns explicitly; validate payload schema before
publish.

### P2 — Audit query initially used invalid SQL (fixed in audit command)

The command used `IN ('...')` inside a Python string with quoting error. The direct
follow-up query used parameters and passed. No application defect.

## Verified passes

- `go test -count=1 ./...`: pass.
- `go vet ./...`: pass.
- Linux amd64 release build: pass.
- Static execution caller allowlist includes the guard delegation.
- Guard blocks stale/missing ownership before exchange network calls.
- V2 process running with unique instance ID.
- `DOCTOR_OK`, OKX/Telegram ready, open orders 0.
- `RECONCILE_CLEAN`, unknown/manual checks 0, four positions preserved.
- Operator halt false only after explicit canary confirmation.
- No order placed during cutover or audit.
- Rollback binary/config and cutover backup retained.
- Local `.env` mode `600`; no secret values printed.

## Required remediation order

1. Fix lease renewal evidence and alert on stale DB lease.
2. Wire outbox worker + Supabase/R2 publishers into runtime composition.
3. Provision/test real cloud endpoints without exposing credentials.
4. Replace wrapper/timer arrangement with managed V2 systemd service.
5. Repeat audit and only then consider expanding execution scope.

---

# Re-audit after P0/P1 remediation — 2026-07-20 09:21 UTC

## Remediation deployed

- Commit `ad6e9bb` adds scheduler-scoped ownership and a 30-second renewal loop.
- Renewal loss cancels the scheduler context and fails closed.
- Same-instance guard acquisition reuses the active fence instead of advancing it.
- Durable outbox worker and explicit Supabase conflict mapping are composed into the
  scheduler runtime.
- Cloud credentials are read only from environment and missing partial configuration
  fails startup.
- Release SHA-256:
  `75792db28943bfc87e6bfafafdd114f4352581bba4adab4253c2734f0ca43c21`.

## Re-audit evidence

- Full Go tests, vet, Linux build and diff check: pass.
- Operator halt: active.
- Doctor blocker is only the intentional operator halt.
- Reconcile: clean; open/unknown/manual checks 0; four positions retained.
- One V2 scheduler process.
- Fence stable at `3`; expiry renewed every 30 seconds:
  - `09:19:44` expiry `09:21:14`
  - `09:20:14` expiry `09:21:44`
  - `09:20:44` expiry `09:22:14`
- At observation `09:21:00`, lease expiry was 74 seconds in the future.
- No order was placed.

## Finding status

### P1 lease renewal — CLOSED

Continuous renewal is running and evidenced. Network guards reuse the process-owned
fence. Renewal loss cancels scheduler execution.

### P0 cloud runtime wiring — CODE CLOSED / PRODUCTION DELIVERY BLOCKED

Outbox worker and adapters are now production-composed, but all required VPS cloud
environment variables are absent:

- `SUPABASE_URL`
- `SUPABASE_SERVICE_ROLE_KEY`
- `R2_PRESIGNED_PUT_URL` or R2 credential/provider configuration

No production cloud delivery test can run without these values. Runtime correctly
stays cloud-disabled and does not invent credentials.

### P1 systemd ownership — OPEN

The root-owned healthcheck timer remains active and invokes the V2 wrapper. Disabling
or replacing the unit requires interactive sudo credentials. V1 is not started, but
formal service ownership remains operational debt.

## Final verdict

**Execution remains halted. P1 lease defect is fixed. P0 code wiring is fixed, but
P0 production delivery cannot pass until the operator provisions Supabase/R2
credentials. Systemd P1 also requires privileged operator action.**
