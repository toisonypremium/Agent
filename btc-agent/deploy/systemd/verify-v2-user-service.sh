#!/usr/bin/env bash
set -euo pipefail
root="${BTC_AGENT_APP_DIR:-/home/admin/btc-agent/btc-agent}"
unit=btc-agent-v2.service
[[ "$(systemctl --user is-enabled "$unit")" == enabled ]]
[[ "$(systemctl --user is-active "$unit")" == active ]]
count="$(pgrep -fc "^$root/bin/btc-agent scheduler --config $root/config.yaml$")"
[[ "$count" == 1 ]] || { echo "expected one scheduler, got $count" >&2; exit 1; }
[[ "$(systemctl is-active btc-agent-healthcheck.timer 2>/dev/null || true)" != active ]]
cd "$root"
python3 - <<'PY'
import sqlite3,time
c=sqlite3.connect('data/btc_agent.db'); now=int(time.time())
r=c.execute("select instance_id,fencing_token,expires_at from execution_leases where name='okx-live'").fetchone()
assert r and r[0]=='v2-prod-01' and r[1]>0 and r[2]>now, (r,now)
print('lease',r,'fresh_seconds',r[2]-now)
PY
./bin/btc-agent operator-status --config config.yaml | grep -q 'Operator halt: ACTIVE'
echo systemd_v2_verify=PASS
