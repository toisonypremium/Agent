#!/usr/bin/env bash
set -euo pipefail
root="${AGENT_RELEASE_ROOT:-/opt/agent}"
current="$(readlink -f "$root/current")"
previous="$(find "$root/releases" -mindepth 1 -maxdepth 1 -type d ! -path "$current" -printf '%T@ %p\n' | sort -nr | head -1 | cut -d' ' -f2-)"
[[ -n "$previous" && -x "$previous/agent" ]] || { echo 'no rollback release' >&2; exit 1; }
ln -sfn "$previous" "$root/current.next"; mv -Tf "$root/current.next" "$root/current"
systemctl restart agent.service
"$(dirname "$0")/health-check.sh"
