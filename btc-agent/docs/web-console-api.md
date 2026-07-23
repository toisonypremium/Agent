# Web Console API v1

## Authority

The API is read-only. It is served by the separate `web-console` process, which
opens the already-existing runtime database using `storage.OpenReadOnly`.

There are no API routes for order placement, cancellation, halt resume,
configuration, arbitrary reports, shell commands or generic RPC.

## Envelope

Every successful versioned response uses:

```json
{
  "schema_version": 1,
  "generated_at": "2026-07-23T22:00:00Z",
  "freshness": {"state": "fresh", "age_seconds": 0},
  "data": {},
  "warnings": []
}
```

Errors return a safe machine-readable `code`; paths, SQL details, environment
values and stack traces are never exposed.

## Endpoints

| Method | Path | Data | Limits |
|---|---|---|---|
| `GET` | `/healthz` | process liveness | no runtime detail |
| `GET` | `/api/v1/overview` | halt, market, lease and paper summary | typed allowlist |
| `GET` | `/api/v1/paper/scorecard` | lifecycle scorecard | paper evidence only |
| `GET` | `/api/v1/paper/orders?limit=1..100` | newest paper orders | hard maximum 100 |
| `GET` | `/api/v1/events?limit=1..100` | sanitized pending event metadata | payload JSON intentionally excluded |

Security headers include CSP `default-src 'none'`, `frame-ancestors 'none'`,
`X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy:
no-referrer`, and `Cache-Control: no-store`.

## Deployment status

This contract is local development only. The process is not installed on the
VPS and is not reachable through any gateway. OIDC/identity proxy and the
future halt-only workflow remain deferred until their dedicated authorization
and staging tests exist.
