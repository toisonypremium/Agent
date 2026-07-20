#!/usr/bin/env bash
set -euo pipefail
execute=false; [[ "${1:-}" == '--execute' ]] && execute=true
items=("btc-agent-v1.service" "/opt/btc-agent-v1" "/etc/cron.d/btc-agent-v1")
for item in "${items[@]}"; do echo "would remove: $item"; done
$execute || { echo 'dry-run only; pass --execute after V2 health/reconcile/backup approval'; exit 0; }
"$(dirname "$0")/health-check.sh"
if systemctl is-active --quiet btc-agent-v1.service; then echo 'V1 still running; refusing cleanup' >&2; exit 1; fi
[[ -n "${AGENT_CLEANUP_APPROVED:-}" ]] || { echo 'AGENT_CLEANUP_APPROVED required' >&2; exit 1; }
rm -rf /opt/btc-agent-v1 /etc/cron.d/btc-agent-v1
rm -f /etc/systemd/system/btc-agent-v1.service
systemctl daemon-reload
