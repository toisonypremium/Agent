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

---

# Final systemd and durability re-audit — 2026-07-20 11:57 UTC

## Closed findings

- **P1 systemd ownership: CLOSED.** `btc-agent-v2.service` is the authoritative,
  enabled user service. The legacy root timer is inactive and disabled. Controlled
  restart and full VPS reboot both produced exactly one scheduler process.
- **P1 fencing continuity: CLOSED.** Graceful release now expires the ownership row
  instead of deleting it, preserving the monotonic sequence. Fence advanced `1 -> 2`
  on controlled restart and `2 -> 3` after reboot.
- **P1 R2 contract: CODE CLOSED.** Production SigV4 signing, checksum, deterministic
  object keys and strict credential-set validation are implemented and tested.
- **P2 Supabase conflict mapping: CODE CLOSED.** Event/table/conflict mappings are
  explicit rather than universal.
- **Outbox crash recovery: CLOSED.** Stale processing claims recover after timeout;
  active claims are not stolen.

## Deployment evidence

- Commit `09bd78e` preserves fencing across graceful restart.
- Commit `1ba46e5` packages the proven V2 unit plus install/verify scripts.
- Production binary SHA-256:
  `bd50a15c9604e5dc474b3f3c7e140d5e066d98669bb7b2cf73b7368682c716cf`.
- Post-reboot service PID `1349`, restart count `0`, owner `v2-prod-01`, fence `3`.
- Operator halt active; doctor blocker only the halt; reconcile clean; open/unknown/
  manual checks `0`; no order placed.

## Deferred external activation

Real Supabase/R2 delivery remains intentionally deferred. This is an external service
activation gate, not an open code or systemd defect. Unrestricted live trading is not
approved while operator halt is active and shadow observation is incomplete.

## Release follow-up required after `5276c1c`

This historical audit does not prove the runtime state of later releases. Before
cutover or any authority change, repeat
`btc-agent/docs/production-verification-checklist.md` against the reviewed release and
record a new audit section with the exact SHA, lease, reconciliation, outbox/cloud and
dry-run evidence. Keep this document historical; do not replace old evidence with
unverified current claims.

---

# Halted shadow release verification — 2026-07-21

## Release evidence

- Source commit: `84cd6a92422033e08745ff0e659c36f693ad6da7`.
- Deployed binary was built from the equivalent pre-push tree and has SHA-256
  `62032762505edf9e49b0ea0a49457737437f00b37f62618f9239740076ff5107`.
- CI run `29825446359` passed format, full Go tests, vet and build.
- Pre-release binary, config and consistent SQLite backup were retained under
  `backups/pre-887ac69-20260721T105039Z`; checksums were recorded on the VPS.

## Observation evidence

Observation from 2026-07-21 10:52 UTC through at least 11:16 UTC proved:

- `btc-agent-v2.service` active with one scheduler process and zero restarts;
- operator halt active throughout;
- owner `v2-prod-01`, fencing token `23`, lease continuously fresh;
- doctor blocked only on the intentional operator halt;
- reconciliation clean with local open, unknown, remote pending, remote-only,
  identity conflicts and manual checks all `0`;
- order submissions, order events and position events after deploy all `0`;
- outbox dead-letter `0`, critical runtime events `0`, critical log hits `0`;
- forced synthetic simulation passed with `exchange_calls=0`;
- current market authority remained `SCOUT/WATCH/MARKDOWN`, desired orders `0`.

## Verdict

**APPROVED_MONITORING / APPROVED_HALTED_SHADOW.** The release is approved for the
current halted monitoring state. It is not approval to clear halt, run a real-order
canary, increase sizing or enable unrestricted live trading. No real order was placed.
V1 rollback artifacts must remain until a separately approved rollback window closes.
