#!/usr/bin/env bash
# Read-only verifier for the supported halted-shadow user runtime.
set -euo pipefail

root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
runtime="$root/runtime"
config="$runtime/config.yaml"
agent="$root/immutable/current/agent"

[[ "$(systemctl --user is-enabled btc-agent-immutable.service)" == enabled ]]
[[ "$(systemctl --user is-active btc-agent-immutable.service)" == active ]]
[[ "$(systemctl --user is-active btc-agent-immutable-observe.timer)" == active ]]
[[ "$(systemctl --user is-active btc-agent-immutable-backup.timer)" == active ]]
[[ "$(pgrep -fc "^$agent scheduler --config $config$")" == 1 ]]
"$agent" config-check --config "$config" | grep -q 'mode=paper paper_trading=true real_trading_enabled=false'
"$agent" operator-status --config "$config" | grep -q 'Operator halt: ACTIVE'
AGENT_HEARTBEAT_FILE="$runtime/reports/scheduler_heartbeat_latest.json" AGENT_HEARTBEAT_MAX_AGE_SECONDS=300 bash "$root/immutable/health-check.sh"
python3 - "$runtime/data/btc_agent.db" <<'PY'
import sqlite3, sys, time
conn = sqlite3.connect(sys.argv[1])
try:
    assert conn.execute("pragma quick_check").fetchone()[0].lower() == "ok"
    lease = conn.execute("select instance_id,fencing_token,expires_at from execution_leases where name=?", ("okx-live",)).fetchone()
    assert lease and lease[0] == "immutable-shadow-01" and lease[1] > 0 and lease[2] > int(time.time())
finally:
    conn.close()
PY
latest="$(find "$runtime/backups" -maxdepth 1 -type f -name 'snapshot-*.tar.gz' -printf '%T@ %p\n' | sort -nr | head -1 | cut -d' ' -f2-)"
[[ -n "$latest" ]]
"$root/immutable/verify-backup.sh" "$latest" >/dev/null
! systemctl --user --failed --no-legend | grep -q .
printf 'immutable_runtime_verify=PASS release=%s\n' "$(readlink -f "$root/immutable/current")"
