# LLM operations runbook

## Authority boundary

- Web + local SQLite are the primary operational view.
- Supabase is a cloud read model. R2 stores immutable audit artifacts.
- Telegram is an optional fallback channel.
- LLM output is narrative or a constrained proposal. It does not override operator halt, doctor, reconciliation, ownership/fencing, stale-data, exchange-filter, inventory, capital, or protection locks.
- Hermes refreshes its protection snapshot before an execution-consumable LLM decision. Missing, malformed, unreadable, or unwritable protection state fails closed before the call; persistence failure emits `HERMES_PROTECTION_SNAPSHOT_FAILED`.
- Web refresh never calls an LLM and has no direct BUY/SELL/CANCEL route.

## Scheduling

- `ai.hermes_interval_minutes: 0` disables periodic Hermes narrative cycles.
- Explicit `hermes-cycle`, `--run-now`, and configured event-driven requests remain available.
- In canary/autonomous mode, the supervisor `pre_execution` path is the only producer of an execution-consumable Hermes decision.
- Scheduled narrative does not refresh the execution decision artifact.
- Telegram opening/midday/closing/digest timers and command polling are absent when Telegram notifications are disabled.

## Token policy

Purpose-specific defaults:

| Purpose | Max completion tokens |
|---|---:|
| `hermes_operator_decision` | 900 |
| `hermes_narrative` | 1400 |
| `ai_watch` | 1200 |
| `research_brief` | 2000 |
| `expert_research` | 3200 |
| `interactive_question` | 1600 |
| `scheduler_telegram_formatter` | 2000 |

Override through `ai.max_tokens_by_purpose`. `ai.max_tokens` is legacy fallback only. Malformed/truncated structured output fails closed; no automatic retry with a larger cap exists.

## Usage telemetry

Local SQLite stores metadata only:

- request ID, timestamp, purpose, trigger, model
- prompt/completion/total token counts when returned by the provider
- `usage_available=false` when the provider omits usage
- latency, status, bounded error class, normalized state hash

Never store raw prompts, user text, response content, API headers, keys, tokens, or credential-bearing URLs.

Supabase receives idempotent `llm_usage_events`. R2 receives:

```text
llm-usage/YYYY/MM/DD/events/<request-id>.json
llm-usage/YYYY/MM/DD/summary-<aggregate-hash>.json
```

Outbox retries use bounded exponential backoff and dead-letter after the configured limit. Cloud failure does not grant execution authority and does not stop deterministic safety processing.

## Dashboard checks

Review the `llm_usage` and `rui_ro` domains in `/api/v3/bootstrap`:

- calls and skipped calls today
- prompt/completion/total tokens
- failed calls and usage unavailable
- repeated state hashes
- operator halt, Hermes demotion and mode
- reconciliation remote-only/conflicts/discovery failure
- execution lease owner/fencing/expiry
- outbox pending/dead-letter
- protection and capital locks
- position provenance: Hermes execution-owned versus legacy account-observed

Estimated cost remains unknown unless an explicit pricing configuration is added.

## Incident procedures

### Token spike

1. Group usage by purpose/model and inspect repeated state hashes.
2. Confirm `hermes_interval_minutes` and Telegram flags.
3. Check whether state-hash reservation is producing `DECISION_STILL_FRESH`/`STATE_IN_FLIGHT` skips.
4. Check pending runtime events for `HERMES_PROTECTION_SNAPSHOT_FAILED`; keep exposure increases blocked until SQLite protection persistence is healthy.
5. Disable periodic narrative with interval `0` through reviewed runtime configuration if needed.
6. Do not disable deterministic doctor, reconciliation, ownership, protection, or risk gates.

### Provider omits usage

1. Confirm calls have `usage_available=false`, not measured zero.
2. Use call count/latency for operational monitoring.
3. Do not infer billing totals as measured usage.

### Duplicate state hash

1. Check `llm_call_reservations` status and outbox idempotency.
2. Confirm only supervisor `pre_execution` writes `hermes_shadow_decision_latest.json` in executable modes.
3. Keep operator halt active if duplicate execution artifacts or stale decisions are suspected.

### Supabase/R2 unavailable

1. Inspect local outbox pending/dead-letter counts.
2. Restore cloud connectivity without changing execution authority.
3. Local SQLite remains the usage ledger; do not delete/re-enqueue by hand without preserving idempotency keys.

### Disable all LLM calls

Set `ai.enabled: false` through reviewed runtime configuration. Deterministic analysis, Web monitoring, doctor, reconciliation and hard execution safety remain active; no LLM decision is created.
