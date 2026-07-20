# V2 Migration Report

Implemented foundations: architecture audit, application ports, lease/fencing core,
SQLite lease persistence, durable outbox and retries, typed heartbeat model, Supabase
read-model migration/RLS, deterministic R2 keys, read-only dashboard, CI and VPS
systemd/backup/rollback/cleanup guardrails. Trading strategy behavior was not changed.
Pending before production cutover: wire ownership into every executor, implement
cloud publishers, authenticate live dashboard data, complete shadow comparison,
production rehearsal and operator approval.
