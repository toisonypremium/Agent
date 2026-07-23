# BTC Agent — Reusable Project Prompts

Load `.claude/MASTER_PROMPT.md` first. Each prompt below is bound to real repository modules.

## 1. Audit repository

**Mục tiêu:** Audit architecture, authority boundaries, state, tests, and production risk without modifying files.

**File/module liên quan:** `main.go`, `cli.go`, `README.md`, `internal/config/config.go`, `internal/agent1`, `internal/agent2`, `internal/liveguard`, `internal/storage`, `internal/exchange/live`, `scheduler.go`, `telegram_commands.go`.

**Quy tắc cấm:** No edits; no secret values; no live commands; do not infer execution authority from reports alone.

**Output bắt buộc:** Project identity; entrypoint; module/call-path map; config/env names; integrations; tests; Critical/High/Medium/Low risks; evidence as exact files and symbols.

**Điều kiện dừng:** Stop when every claim has repository evidence or is explicitly marked unknown.

## 2. Plan P0/P1/P2

**Mục tiêu:** Turn audited findings into safety-prioritized work.

**File/module liên quan:** Findings plus affected packages, especially `internal/liveguard`, `cmd_supervisor.go`, `internal/storage`, and scheduler files.

**Quy tắc cấm:** Do not rank profit/sizing improvements above loss of ownership, reconciliation, data health, credentials, or order authority. Do not edit.

**Output bắt buộc:** One-line problem, evidence, impact, smallest patch, acceptance test, rollback, dependencies for each P0/P1/P2 item.

**Điều kiện dừng:** Each item must be independently verifiable; reject evidence-free tickets.

## 3. Safe patch

**Mục tiêu:** Apply one minimal, reviewable change while retaining safety invariants.

**File/module liên quan:** Only the proven call path and its tests.

**Quy tắc cấm:** No adjacent refactor, new dependency, config activation, secret, real order, or hidden scope expansion.

**Output bắt buộc:** Assumptions; selected skill; patch plan; exact diff scope; focused/full checks; rollback; residual risk.

**Điều kiện dừng:** Stop if authority or desired behavior is ambiguous, or if a focused test cannot express success.

## 4. Bug fix

**Mục tiêu:** Reproduce, localize, repair, and guard a defect.

**File/module liên quan:** Start at failing report/test/log, then trace to the owning function; common seams include `internal/liveguard/*_test.go`, `internal/storage/*_test.go`, and root command tests.

**Quy tắc cấm:** No symptom-only workaround, unrelated cleanup, real exchange reproduction, or deletion of safety checks.

**Output bắt buộc:** Reproduction; root cause; failing test; smallest fix; passing regression; broader validation; rollback.

**Điều kiện dừng:** The test must fail for the original cause and pass for the fix.

## 5. Test/check

**Mục tiêu:** Select checks proportional to the patch blast radius.

**File/module liên quan:** `*_test.go`, `Makefile`, README verification section; add race tests for `internal/storage`, scheduler, or shared state.

**Quy tắc cấm:** Do not call OKX order endpoints or expose real env values. Do not accept build-only proof for behavioral changes.

**Output bắt buộc:** Test matrix; command/result; skipped checks with reason; negative safety cases; remaining gaps.

**Điều kiện dừng:** Focused regression, full Go tests, vet/build/diff checks appropriate to the change pass.

## 6. Code review

**Mục tiêu:** Review a diff against request, safety, correctness, concurrency/state, security, and operability.

**File/module liên quan:** Changed files plus callers/callees and tests; compare with `README.md` invariants.

**Quy tắc cấm:** Do not edit during review; do not report style preferences as defects; do not suppress uncertain high-risk findings.

**Output bắt buộc:** Findings ordered Critical→Low with file/symbol evidence; spec compliance; missing tests; approve/block verdict.

**Điều kiện dừng:** Every finding has concrete failure mode and remediation.

## 7. Security review

**Mục tiêu:** Threat-model secrets, untrusted inputs, external APIs, execution authority, and local state.

**File/module liên quan:** `internal/config/config.go`, `internal/exchange/live/okx.go`, `internal/notify/telegram.go`, `telegram_commands.go`, `internal/llm/client.go`, `scripts/check-secrets.sh`, `.gitignore`.

**Quy tắc cấm:** Never print secret contents; no credential validation by placing orders; no widening Telegram or LLM authority.

**Output bắt buộc:** Assets; trust boundaries; threat/impact/evidence; existing controls; smallest mitigation; secret-rotation note if exposure is proven.

**Điều kiện dừng:** All credential and command-input paths are accounted for.

## 8. Deploy/runtime review

**Mục tiêu:** Assess production readiness and service health with rollback-first operations.

**File/module liên quan:** `deploy/systemd/btc-agent-immutable.service`, `systemctl --user restart btc-agent-immutable.service`, `scheduler.go`, `scheduler_lock.go`, `scheduler_heartbeat.go`, `cmd_live.go`, `cmd_supervisor.go`, `reports/live_doctor_latest.json`.

**Quy tắc cấm:** No deployment without passing checks and explicit task authority; no live enablement; no automatic resume from halt; no test order.

**Output bắt buộc:** Preflight; artifact/commit identity; backup; restart; health; doctor/supervisor; order delta; rollback trigger and command.

**Điều kiện dừng:** Service healthy and no unexpected order/state/authority change, or rollback executed.

## 9. Trading safety review

**Mục tiêu:** Prove a trading/order/signal patch cannot bypass market and execution gates.

**File/module liên quan:** `internal/agent1/analyst.go`, `internal/agent2/planner.go`, `internal/liveguard/guard.go`, `preflight.go`, `risk_governor.go`, `hermes_execution.go`, `exit_manager.go`, `hermes_production.go`.

**Quy tắc cấm:** No futures/leverage/short/market BUY; no stop-loss SELL below average entry; no weakened caps, ownership, reconcile, stale-data, or exchange-filter checks.

**Output bắt buộc:** Authority chain; allowed/blocked cases; invariant test matrix; dry-run evidence; residual capital risk.

**Điều kiện dừng:** Negative tests prove every relevant bypass remains blocked.

## 10. Telegram alert review

**Mục tiêu:** Verify readable Vietnamese, read-only authority, redaction, keyboard behavior, and deduplication.

**File/module liên quan:** `internal/notify/telegram.go`, `telegram_manager.go`, `internal/telegramreport`, `internal/usertext/vietnamese.go`, `telegram_commands.go`.

**Quy tắc cấm:** No execution/control command; no token/chat ID leakage; no uncontrolled notification flood; no unnecessary real messages.

**Output bắt buộc:** Command mapping; authority proof; representative rendered text; terminology/redaction/chunking tests; menu hide behavior.

**Điều kiện dừng:** All interactive paths remain information-only and tests cover changed output.

## 11. State recovery review

**Mục tiêu:** Validate SQLite/reports/restart behavior after partial failure or process restart.

**File/module liên quan:** `internal/storage/sqlite.go`, ledger/order/runtime-state files, `cmd_reconcile.go`, `scheduler_lock.go`, `reports/exit_peak_tracker.json`.

**Quy tắc cấm:** No DB replacement without backup; no write under open cursor; no synthetic ownership; no auto-resume; no destructive migration without rollback.

**Output bắt buộc:** State sources of truth; recovery sequence; idempotency; cursor/transaction audit; restart tests; backup/restore steps.

**Điều kiện dừng:** Recovery is fail-closed and repeatable without duplicate orders or lost ownership.
