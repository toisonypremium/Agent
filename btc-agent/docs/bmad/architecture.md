# Architecture baseline

## Luồng thẩm quyền

```text
Public data -> research/context
OHLCV + microstructure -> Agent 1 BTC gate -> Agent 2 asset plan
Plan -> liveguard preflight/risk/reconcile -> managed order engine
Dashboard -> read-only reports/SQLite
```

## Ranh giới

- `research`, `freeapi`, Hermes: context/report.
- `agent1`, `agent2`: permission và plan.
- `liveguard`: final assertion, timeout, reconcile, persistence fail-closed.
- `storage`: ledger và runtime evidence.
- `web`: read-only projection.

## Quality attributes

- Fail closed khi thiếu dữ liệu hoặc trạng thái không xác định.
- Single writer cho live order lifecycle.
- Idempotency bằng managed key/client order ID.
- Timeout cho network operations.
- Atomic binary replacement và backup trước restart.
- Không để secret vào report, log hoặc repository.

## ADR bắt buộc

Mỗi thay đổi ảnh hưởng authority, order lifecycle, data source hoặc production topology phải có ADR ngắn: bối cảnh, quyết định, phương án loại bỏ, tác động, rollback.
