# LLM operations runbook

## Authority boundary

- Local SQLite is the primary operational record.
- Telegram is an optional notification and read-only status channel.
- LLM output is narrative or a constrained proposal. It does not override operator halt, doctor, reconciliation, ownership/fencing, stale-data, exchange-filter, inventory, capital, or protection locks.
- Hermes refreshes its protection snapshot before an execution-consumable LLM decision. Missing, malformed, unreadable, or unwritable protection state fails closed before the call; persistence failure emits `HERMES_PROTECTION_SNAPSHOT_FAILED`.

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

1. Check `llm_call_reservations` status.
2. Confirm only supervisor `pre_execution` writes `hermes_shadow_decision_latest.json` in executable modes.
3. Keep operator halt active if duplicate execution artifacts or stale decisions are suspected.

### Disable all LLM calls

Set `ai.enabled: false` through reviewed runtime configuration. Deterministic analysis, doctor, reconciliation and hard execution safety remain active; no LLM decision is created.
