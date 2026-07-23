#!/usr/bin/env bash
# Read-only production preflight. It never changes halt, credentials, config,
# exchange state, or service state.
set -euo pipefail

service="${AGENT_SERVICE_NAME:-agent.service}"
root="${AGENT_RELEASE_ROOT:-/opt/agent}"
state="${AGENT_DATA_DIR:-/var/lib/agent}"
config="${AGENT_CONFIG_PATH:-/etc/agent/config.yaml}"
envfile="${AGENT_ENV_PATH:-/etc/agent/agent.env}"
heartbeat="${AGENT_HEARTBEAT_FILE:-$state/runtime/scheduler_heartbeat_latest.json}"

command -v systemctl >/dev/null || { echo 'BLOCKED: systemctl unavailable; run on the Linux production host' >&2; exit 2; }
[[ -L "$root/current" && -x "$root/current/agent" ]] || { echo 'BLOCKED: immutable current release missing' >&2; exit 1; }
[[ -f "$config" && -f "$envfile" ]] || { echo 'BLOCKED: protected config/environment missing' >&2; exit 1; }
for path in "$config" "$envfile"; do
  mode="$(stat -c '%a' "$path")"
  [[ "$mode" =~ ^[0-7][0-7][0-7]$ && "${mode:1:2}" == 00 ]] || { echo "BLOCKED: $path must be owner-only (mode=$mode)" >&2; exit 1; }
done
systemctl is-enabled --quiet "$service"
systemctl is-active --quiet "$service"
processes="$(pgrep -fc '^/opt/agent/current/agent scheduler --config /etc/agent/config.yaml$' || true)"
[[ "$processes" == 1 ]] || { echo "BLOCKED: expected one scheduler, got $processes" >&2; exit 1; }
for legacy in btc-agent-v1.service btc-agent-v2.service btc-agent-scheduler.service; do
  if systemctl is-active --quiet "$legacy" 2>/dev/null; then
    echo "BLOCKED: legacy service active: $legacy" >&2
    exit 1
  fi
done
AGENT_HEARTBEAT_FILE="$heartbeat" bash "$(dirname "$0")/health-check.sh"
printf 'runtime_verify=PASS service=%s release=%s scheduler_count=%s\n' "$service" "$(readlink -f "$root/current")" "$processes"
