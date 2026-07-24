# Progress

## Done
- Guarded DCA allocation, caps, lifecycle, Web read models, immutable rollout,
  and final activation work are committed and deployed.
- Long-running operator controls added under `.agents/skills/trading-long-running`.

## In progress
- Live runtime monitoring remains active. Do not alter runtime configuration,
  operator halt, or execution settings without explicit operator authorization.

## Next
- Monitor scheduler, observer, reconciliation, DCA safety state, and any order
  lifecycle evidence.

## Notes
- No credentials, tokens, or account secrets belong in this file.
- `AGENT_STOP` halts non-read-only work. `STEER.md` is preserved operator input.
