#!/usr/bin/env bash
set -euo pipefail
heartbeat="${AGENT_HEARTBEAT_FILE:-/var/lib/agent/runtime/scheduler_heartbeat_latest.json}"
max_age="${AGENT_HEARTBEAT_MAX_AGE_SECONDS:-300}"
[[ -s "$heartbeat" ]] || { echo "heartbeat missing: $heartbeat" >&2; exit 1; }
python3 - "$heartbeat" "$max_age" <<'PY'
import datetime,json,sys
p,max_age=sys.argv[1],int(sys.argv[2]); d=json.load(open(p)); raw=d.get('generated_at') or d.get('GeneratedAt')
if not raw: raise SystemExit('heartbeat timestamp missing')
t=datetime.datetime.fromisoformat(raw.replace('Z','+00:00')); age=(datetime.datetime.now(datetime.timezone.utc)-t).total_seconds()
if age<0 or age>max_age: raise SystemExit(f'heartbeat stale age={age:.0f}s')
print(f'healthy heartbeat_age={age:.0f}s')
PY
