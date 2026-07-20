# Database Schema

Supabase is a history/read model. OKX remains authoritative for balances, positions,
orders and fills. Migrations are versioned under `supabase/migrations`; dashboard RLS
is under `supabase/policies` and grants authenticated read only.

Uniqueness on correlation, client order, idempotency, exchange order/fill and object
keys prevents duplicate synchronization. High-volume heartbeat data should retain
7–14 days; orders, fills and audit logs remain long term; large reports live in R2.
