#!/usr/bin/env bash
set -euo pipefail

root="$(mktemp -d)"
cleanup() { rm -rf "$root"; }
trap cleanup EXIT
fresh="$root/fresh.json"
stale="$root/stale.json"
python3 - "$fresh" "$stale" <<'PY'
import datetime, json, sys
now = datetime.datetime.now(datetime.timezone.utc)
for path, value in ((sys.argv[1], now), (sys.argv[2], now - datetime.timedelta(minutes=10))):
    with open(path, "w", encoding="utf-8") as f:
        json.dump({"generated_at": value.isoformat().replace("+00:00", "Z")}, f)
PY
AGENT_HEARTBEAT_FILE="$fresh" AGENT_HEARTBEAT_MAX_AGE_SECONDS=300 bash "$(dirname "$0")/health-check.sh" | grep -q '^healthy heartbeat_age='
if AGENT_HEARTBEAT_FILE="$stale" AGENT_HEARTBEAT_MAX_AGE_SECONDS=300 bash "$(dirname "$0")/health-check.sh" >/dev/null 2>&1; then
  echo 'stale heartbeat unexpectedly passed' >&2
  exit 1
fi
printf 'healthcheck_drill=PASS\n'
