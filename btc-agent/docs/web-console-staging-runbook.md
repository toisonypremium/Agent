# Web Console staging runbook

## Scope and hard boundary

This runbook prepares a local/staging review only. It does **not** authorize a
VPS install, public exposure, service restart, runtime configuration change,
operator-halt change, exchange operation, or canary/live operation.

## Build

```bash
cd btc-agent/web
npm ci
npm run test
npm run lint
npm run build
cd ..
go test ./internal/webconsole ./cmd/web-console
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/web-console ./cmd/web-console
```

## Local temporary fixture only

Use a disposable database created with `storage.Open`; never point the process
at a production database during development. The executable requires an
existing database and opens it with `mode=ro` and `query_only=ON`.

```bash
BTC_AGENT_WEB_DB_PATH=/absolute/path/to/fixture.db \
BTC_AGENT_WEB_STATIC_DIR="$PWD/web/dist" \
BTC_AGENT_WEB_LISTEN_ADDR=127.0.0.1:8787 \
./bin/web-console
```

The executable rejects wildcard, hostname, and public IP addresses. Verify:

```bash
curl -i http://127.0.0.1:8787/healthz
curl -i http://127.0.0.1:8787/api/v1/overview
curl -i http://127.0.0.1:8787/.env             # must be 404
curl -i http://127.0.0.1:8787/secret.txt       # must be 404
curl -i http://127.0.0.1:8787/api/v1/orders    # must be 404
```

## Future gateway and deployment prerequisites

Before any non-loopback staging exposure, require a separate approval and:

1. Identity-aware TLS gateway with OIDC audience/issuer/role validation.
2. Private proxy-to-loopback networking; no direct public listener.
3. Explicit security review of CSRF session model before adding a halt request.
4. Immutable separate artifact/service with no systemd control privilege.
5. API/authority negative tests, read-only database check, CSP/header check,
   static allowlist check, backup and scheduler isolation verification.
6. A rollback that removes only the Web Console process and leaves the
   immutable scheduler release, database, halted state and evidence untouched.

## Public Access production checks

The Cloudflare Tunnel process uses a mode-`0600` `EnvironmentFile`; never pass
tunnel tokens on a command line or commit them. Confirm the public hostname
returns a Cloudflare Access login redirect when unauthenticated, then sign in
and verify a successful console read. Before any browser halt test, confirm the
operator halt is already active and record paper/runtime evidence. Exercise
only a negative request (missing CSRF or Access JWT) until the operator
explicitly elects to submit a real one-way halt request.
