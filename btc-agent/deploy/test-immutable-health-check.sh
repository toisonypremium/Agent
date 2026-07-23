#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d "${PWD}/.immutable-health-test.XXXXXX")"
trap 'rm -rf "$root"' EXIT
python3 - "$root/heartbeat.json" <<'PY'
import datetime, json, sys
json.dump({'generated_at': datetime.datetime.now(datetime.timezone.utc).isoformat()}, open(sys.argv[1], 'w'))
PY
AGENT_HEARTBEAT_FILE="$root/heartbeat.json" AGENT_HEARTBEAT_MAX_AGE_SECONDS=300 bash deploy/immutable-health-check.sh | grep -q 'healthy heartbeat_age='
printf 'immutable_healthcheck_drill=PASS\n'
