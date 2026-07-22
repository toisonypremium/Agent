# Circuit Framework research sidecar

## Boundary

Circuit is a deterministic research-only sidecar. The Go core remains canonical for market truth, safety, authority, sizing, and execution. Circuit receives no exchange, Telegram, database, or Hermes MCP credentials and cannot submit proposals or orders.

Allowed research actions: `WATCH`, `NO_TRADE`, `INVESTIGATE`. Every accepted output requires `authority=RESEARCH_ONLY` and `execution_intent=null`.

## Contracts

- `circuit-research-input-v1`: sanitized market/plan projection, data quality, canonical authority, immutable policy, content SHA-256.
- `circuit-research-evidence-v1`: exact input and producer binding, timestamps, provenance, limitations, output SHA-256.
- Unknown fields, stale/expired/future data, malformed JSON, forbidden actions, missing provenance, producer mismatch, or hash mismatch fail closed.

## Operations

```bash
systemctl --user status btc-agent-circuit-research.timer
systemctl --user start btc-agent-circuit-research.service
cat reports/circuit/soak_status.json
```

The timer runs every four hours. An hourly status timer records soak metrics. Validated immutable runs are retained under `reports/circuit/runs` for 30 days. Invalid runs never overwrite `evidence_latest.json`.

## Rollback

```bash
scripts/rollback-circuit-research.sh
```

Rollback disables only the Circuit timer/service. It does not stop scheduler/web, modify production SQLite, or delete historical evidence.

## Pilot gates

After at least 14 days:

- schema-valid runs >= 99%;
- stale accepted = 0;
- production DB writes = 0;
- runtime secret exposure = 0;
- scheduler/web impact = 0;
- failed sidecar unit leakage = 0.

Passing the pilot does not authorize multi-agent mode, UI integration, Hermes prompt ingestion, sizing changes, or execution. Those require a separate plan and explicit operator approval.

## Provenance

Pinned upstream source: `PengZhang64/circuit-framework` commit `7ab0137caef9e09b5666c8fbb1e10353aa4eb4b3`, Apache-2.0.
