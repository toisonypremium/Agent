# Codex re-review handoff — audit remediation

## Finding disposition

| Audit finding | Disposition |
|---|---|
| Capital allocator division by zero | Already protected by `totalWeight <= 0`; added regression coverage |
| `firstCandidate` asset-state race | Already protected; added non-active/non-finite regression coverage |
| Reserve/layer bounds | Already bounded; extended NaN/Inf validation and fuzzing |
| Reconcile errors silent | Rejected as stated: unknown is safety-blocking and capital-reserving; added critical deduplicated runtime event, no unsafe auto-cancel |
| YAML secrets | Exchange/LLM already env-only; removed Telegram YAML fallback, migrated production token to protected env, added rotation runbook |
| Threshold calibration leakage | Fixed with chronological purged 60/20/20 profile/calibration/validation splits |
| Core signal walk-forward gap | Added Flow/BTC permission walk-forward with embargo and deterministic tests |
| FailedBreakdown dead logic | Kept strict production rule; added shadow candidate detector for OOS comparison |
| FOMO 98% threshold | Replaced with breakout + ATR-distance logic and boundary tests |
| Duplicate permission functions | Not reproduced; no speculative refactor |
| Ignored persistence marshal errors | Fixed critical market, plan, live order/fill/position, Hermes and operator-change writers |
| Zero-slippage simulation | Added configurable slippage, fee and partial-fill model; defaults remain backward compatible |
| AI override negative test | Already exists (`TestGenerateFallsBackOnOverride`) |
| Lifecycle/live path coverage | Existing state matrix + live manager simulation; added allocator/candidate boundaries |

## Review priorities

1. Verify no calibration sample crosses profile/calibration/validation embargoes.
2. Verify final threshold verdict uses validation rows only.
3. Verify realistic fills conserve quantity and never improve cost under positive slippage/fees.
4. Verify candidate FailedBreakdown remains shadow-only and does not alter production bias.
5. Verify unknown reconcile remains active/reserved, blocks execution and never auto-cancels.
6. Verify all secret values are environment-only and no report/log prints them.
7. Verify persistence marshal failures cannot commit partial rows.
8. Verify `override_engine=true` always falls back to deterministic output.

## Safety boundaries

- No direct order API/UI added.
- No authority activation.
- Circuit remains `RESEARCH_ONLY`.
- Mutation flags remain OFF.
- Unknown exchange outcome remains fail-closed.
- Research calibration never updates production config.

## Tests required before approval

```bash
go test -count=1 ./...
go test -race ./internal/liveguard ./internal/storage ./internal/backtest ./internal/flow ./internal/agent1 ./internal/config .
go test -fuzz=FuzzOperatorChange -fuzztime=30s ./internal/operatorchange
go test -fuzz=FuzzConfigValidation -fuzztime=30s ./internal/config
node web/test_dashboard_state.js
node web/test_dashboard_escape.js
node web/test_dashboard_contract.js
```

## Secret operational note

Production YAML token was removed and runtime doctor passed using a `0600` systemd
environment file. Provider-side Telegram token rotation still requires operator
confirmation if the old plaintext token may have existed in backup/history.
