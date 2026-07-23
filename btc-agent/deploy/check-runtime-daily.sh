#!/usr/bin/env bash
# Read-only daily operational digest for an immutable halted-paper runtime.
set -euo pipefail
root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
runtime="$root/runtime"
config="$runtime/config.yaml"
agent="$root/immutable/current/agent"
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
status=PASS
reason=()
if ! verification="$("$root/immutable/verify-runtime.sh" 2>&1)"; then status=FAIL; reason+=("runtime verifier failed: ${verification//$'\n'/ }"); fi
scorecard="$runtime/reports/paper_scorecard_latest.md"
readiness=UNKNOWN
if [[ -f "$scorecard" ]]; then readiness="$(awk -F': ' '/^Readiness:/{print $2; exit}' "$scorecard")"; else status=FAIL; reason+=("paper scorecard missing"); fi
latest="$(find "$runtime/backups" -maxdepth 1 -type f -name 'snapshot-*.tar.gz' -printf '%T@ %p\n' | sort -nr | head -1 || true)"
backup_age=unknown
if [[ -n "$latest" ]]; then backup_age="$(( $(date +%s) - ${latest%% *} ))s"; else status=FAIL; reason+=("backup missing"); fi
available_kb="$(df -Pk "$runtime" | awk 'NR==2{print $4}')"
if (( available_kb < 1048576 )); then status=FAIL; reason+=("available disk below 1GiB"); fi
printf 'runtime_daily_digest time=%s status=%s paper_readiness=%s backup_age=%s available_kb=%s reason=%s\n' "$now" "$status" "$readiness" "$backup_age" "$available_kb" "${reason[*]:-}"
[[ "$status" == PASS ]]
