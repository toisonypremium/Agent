# Data Flow

## Critical path

```text
Binance market data -> btc-agent -> deterministic strategy -> risk/preflight
-> execution ownership -> OKX -> local SQLite ledger
```

No Vercel, Supabase, R2, Telegram or LLM dependency is allowed in this path.

## Asynchronous read-model path

```text
Local event -> SQLite durable outbox -> Supabase metadata/read model
                                |-----> R2 reports/artifacts
                                `-----> Telegram alerts
```

OKX wins reconciliation conflicts for balances, positions, orders and fills.
Supabase stores history/read models. SQLite stores cache, outbox and recovery state.
