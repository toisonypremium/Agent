# Progress

## Done
- Guarded DCA allocation, caps, lifecycle, Web read models, immutable rollout,
  and final activation work are committed and deployed.
- Latest source head: `0f99883`; DCA runtime release: `dca-live-3102586`.
- Latest observer soak: iteration 10 PASS; decimal-exact balance invariant,
  observer/API, singleton scheduler, fresh lease, and SQLite all verified.
- Long-running operator controls added under `.agents/skills/trading-long-running`.

## In progress
- Live runtime monitoring remains active. Current verified state at 2026-07-24
  13:09 UTC: `halt=false`, no DCA orders, DCA safety `(errors=0, stale=0, reason="")`.
- Do not alter runtime configuration, operator halt, execution settings, DB, or
  orders without explicit operator authorization.

## Next
- Monitor scheduler, observer, reconciliation, DCA safety state, planner
  `ACTIVE_LIMIT`, and any DCA order lifecycle evidence.
- On an incident, diagnose read-only first; propose tested source changes and
  wait for approval before deployment or resumption actions.

## Notes
- No credentials, tokens, or account secrets belong in this file.
- `AGENT_STOP` halts non-read-only work. `STEER.md` is preserved operator input.
- Existing scheduled observer audit remains read-only; the noisy DCA event cron
  was cancelled.
