#!/usr/bin/env bash
# Records one read-only halted-shadow observation. It never changes config,
# credentials, execution authority, exchange state, or the scheduler service.
set -euo pipefail

root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
runtime="$root/runtime"
release="$root/immutable/current"
out="$runtime/soak/observations.tsv"
max_heartbeat_age="${BTC_AGENT_HEARTBEAT_MAX_AGE_SECONDS:-300}"

mkdir -p "$(dirname "$out")"
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
unit="$(systemctl --user is-active btc-agent-immutable.service 2>/dev/null || true)"
count="$(pgrep -fc "^$release/agent scheduler --config $runtime/config.yaml$" || true)"
health="$(AGENT_HEARTBEAT_FILE="$runtime/reports/scheduler_heartbeat_latest.json" AGENT_HEARTBEAT_MAX_AGE_SECONDS="$max_heartbeat_age" bash "$root/immutable/health-check.sh" 2>&1 || true)"

read -r instance fence remaining integrity <<EOF2
$(python3 - "$runtime/data/btc_agent.db" <<'PY'
import sqlite3, sys, time
conn = sqlite3.connect(sys.argv[1])
try:
    row = conn.execute("select instance_id, fencing_token, expires_at from execution_leases where name=?", ("okx-live",)).fetchone()
    integrity = conn.execute("pragma quick_check").fetchone()[0]
    print("missing 0 -999" if row is None else f"{row[0]} {row[1]} {row[2] - int(time.time())}", integrity)
finally:
    conn.close()
PY
)
EOF2

status=PASS
reason=""
if [[ "$unit" != active || "$count" != 1 || "$instance" != immutable-shadow-01 || "$fence" -le 0 || "$remaining" -le 30 || "$integrity" != ok || "$health" != healthy* ]]; then
  status=FAIL
  reason=runtime_gate
fi
printf '%s\tstatus=%s\tservice=%s\tschedulers=%s\tlease=%s:%s:%s\tdb=%s\thealth=%s\treason=%s\n' "$now" "$status" "$unit" "$count" "$instance" "$fence" "$remaining" "$integrity" "$(printf '%s' "$health" | tr '\n' ' ')" "$reason" >> "$out"
[[ "$status" == PASS ]]
