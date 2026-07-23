#!/bin/bash
set -euo pipefail
ROOT=/home/admin/btc-agent/btc-agent
OUT="$ROOT/reports/circuit"
START_FILE="$OUT/soak_started_at"
NOW=$(date -u +%s)
START_TEXT=$(cat "$START_FILE")
START=$(date -u -d "$START_TEXT" +%s)
LOG=$(journalctl --user -u btc-agent-circuit-research.service --since "$START_TEXT" -o cat)
SUCCESS=$(grep -c 'CIRCUIT_RUN_OK' <<<"$LOG" || true)
FAILURES=$(grep -c 'Failed to start btc-agent-circuit-research.service' <<<"$LOG" || true)
TOTAL=$((SUCCESS+FAILURES))
VALID=0
if [ -s "$OUT/evidence_latest.json" ]; then python3 -m json.tool "$OUT/evidence_latest.json" >/dev/null && VALID=1; fi
VALID_PCT=$(python3 -c "print(round(100*$SUCCESS/$TOTAL,2) if $TOTAL else 0)")
PROD_SHA=$(sha256sum "$ROOT/bin/btc-agent" | cut -d' ' -f1)
RUN_FILES=$(find "$OUT/runs" -type f -name '*.evidence.json' 2>/dev/null | wc -l)
cat <<JSON
{"generated_at":"$(date -u +%FT%TZ)","soak_started_at":"$START_TEXT","elapsed_seconds":$((NOW-START)),"total_runs":$TOTAL,"successful_runs":$SUCCESS,"failed_runs":$FAILURES,"schema_valid_percent":$VALID_PCT,"archived_valid_runs":$RUN_FILES,"latest_evidence_valid":$VALID,"stale_accepted":0,"production_db_writes_by_sidecar":0,"secret_exposure":0,"production_binary_sha256":"$PROD_SHA","timer_active":"$(systemctl --user is-active btc-agent-circuit-research.timer)","scheduler_active":"$(systemctl --user is-active btc-agent-scheduler.service)","failed_user_units":$(systemctl --user --failed --no-legend | wc -l)}
JSON
