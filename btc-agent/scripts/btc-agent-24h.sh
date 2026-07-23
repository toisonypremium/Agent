#!/data/data/com.termux/files/usr/bin/bash
# Deprecated: this loop is intentionally inert. The only supported unattended
# runtime is deploy/systemd/agent.service running `btc-agent scheduler` from an
# immutable release. Keeping this executable as a fail-closed stub prevents an
# old Termux boot/cron entry from becoming a second order authority.
set -euo pipefail

printf '%s\n' 'btc-agent-24h is retired; use the authoritative systemd agent.service scheduler' >&2
exit 64
