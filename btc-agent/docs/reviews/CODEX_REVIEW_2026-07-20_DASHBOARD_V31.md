# Codex review handoff — Dashboard v3.1

## Review objective

Audit production-bound changes for correctness, security and authority-boundary regressions. Prioritize findings over style.

## Implemented scope

- Typed `/api/v3/bootstrap` aggregate (`dashboard-v3.1`).
- Fixed frontend double-unwrapping that hid canonical BTC/capital/plan data.
- Vietnamese dashboard across seven modules.
- Read-only activity chat from `runtime_events`.
- Bot schedule timeline from canonical scheduler heartbeat.
- Hermes fallback when shadow report is missing, with provenance.
- Per-source freshness/status/age metadata.
- Circuit projection remains `RESEARCH_ONLY`, `execution_intent=null`.
- Operator capability/change contracts; mutation flags remain OFF.

## Production evidence

```text
release sha256: a231fe8458de155e189fff52f5f49e3727bdf077aca089022e98ab54865d2bd1
config sha256:  006dcedb37bb34b8300e6d0491a84c7ef652a921d5f6d4fbdd52f7eb9e5bcfe1
schema:         dashboard-v3.1
readiness:      ready
authority:      BLOCKED
real_order:     NOT_APPROVED
mutation:       OFF
operator API:   404
SQLite:         quick_check=ok
```

Observed parity at acceptance:

```text
BTC price:      64430.01 USDT
capital:        4295.68 USDT
activity:       20 entries
schedule:       canonical heartbeat fields present
Hermes source:  canonical fallback with provenance
```

## Verification completed

- `go test -count=1 ./...`: pass.
- Linux `go test -race`: pass.
- `FuzzOperatorChange`: 300,703 executions / 30s, pass.
- Dashboard state/XSS/contract tests: pass.
- Chromium production DOM and screenshots at 1440, 768, 390, 320: pass.
- Rollback artifact verified:
  `/home/admin/btc-agent/backups/dashboard-v31-20260720T052117Z`.

## Files requiring highest scrutiny

- `web_read_model.go`
- `web/app.js`
- `web/index.html`
- `web/index.css`
- `internal/storage/web_activity.go`
- `web_operator_security.go`
- `internal/operatorcapability/model.go`
- `internal/operatorchange/contract.go`
- `internal/storage/operator_changes.go`
- `internal/circuitresearch/*`

## Mandatory review questions

1. Can missing/stale/corrupt reports be rendered as fresh or healthy?
2. Can activity payloads expose secrets, filesystem paths or unsanitized HTML?
3. Does any UI/API create a direct order, autonomous Hermes or Circuit execution path?
4. Can one identity approve an authority/risk increase twice?
5. Can CSRF/idempotency/session replay bypass HALT protection?
6. Does v3.1 disagree with v1/v2 or canonical report semantics?
7. Can schedule rendering infer a run that canonical heartbeat did not publish?
8. Does the read-only activity query mutate `handled_at` or any DB row?
9. Are ETag/fingerprint and source-age semantics stable and truthful?
10. Are there race, SQLite lock or payload-size denial-of-service risks?

## Explicit non-goals

- Do not enable mutation flags.
- Do not approve real orders.
- Do not promote Circuit or ingest Circuit into Hermes.
- Do not change production trading config.
