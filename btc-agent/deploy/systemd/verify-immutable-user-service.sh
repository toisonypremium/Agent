#!/usr/bin/env bash
# Read-only verifier for the immutable halted-shadow user service.
set -euo pipefail

root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
runtime="$root/runtime"
unit=btc-agent-immutable.service
[[ "$(systemctl --user is-enabled "$unit")" == enabled ]]
[[ "$(systemctl --user is-active "$unit")" == active ]]
[[ "$(systemctl --user is-active btc-agent-v2.service 2>/dev/null || true)" != active ]]
[[ "$(pgrep -fc "^$root/immutable/current/agent scheduler --config $runtime/config.yaml$")" == 1 ]]
AGENT_HEARTBEAT_FILE="$runtime/reports/scheduler_heartbeat_latest.json" \
  AGENT_HEARTBEAT_MAX_AGE_SECONDS=300 bash "$root/immutable/health-check.sh"
python3 - "$runtime/data/btc_agent.db" <<'PY'
import sqlite3,sys,time
conn=sqlite3.connect(sys.argv[1])
try:
    row=conn.execute("select instance_id,fencing_token,expires_at from execution_leases where name=?", ("okx-live",)).fetchone()
    now=int(time.time())
    assert row and row[0] == "immutable-shadow-01" and row[1] > 0 and row[2] > now, (row, now)
    assert conn.execute("PRAGMA quick_check").fetchone()[0].lower() == "ok"
finally:
    conn.close()
PY
printf 'immutable_user_service_verify=PASS\n'
