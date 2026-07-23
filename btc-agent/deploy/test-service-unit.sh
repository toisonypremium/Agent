#!/usr/bin/env bash
set -euo pipefail

unit="$(dirname "$0")/systemd/agent.service"
for line in \
  'User=agent' \
  'EnvironmentFile=/etc/agent/agent.env' \
  'ExecStart=/opt/agent/current/agent scheduler --config /etc/agent/config.yaml' \
  'Restart=on-failure' \
  'NoNewPrivileges=true' \
  'ProtectSystem=strict' \
  'ProtectHome=true' \
  'ReadWritePaths=/var/lib/agent /var/log/agent' \
  'UMask=0077'; do
  grep -Fxq "$line" "$unit" || { echo "missing systemd hardening directive: $line" >&2; exit 1; }
done
if grep -Eq '/home/|btc-agent-v2|btc-agent-scheduler' "$unit"; then
  echo 'service unit contains a legacy runtime path' >&2
  exit 1
fi
printf 'service_unit_drill=PASS\n'
