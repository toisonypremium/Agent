#!/usr/bin/env bash
set -euo pipefail
unit_dir="${HOME}/.config/systemd/user"
source_unit="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/btc-agent-v2.service"
mkdir -p "$unit_dir"
install -m 0600 "$source_unit" "$unit_dir/btc-agent-v2.service"
systemctl --user daemon-reload
systemd-analyze --user verify "$unit_dir/btc-agent-v2.service"
systemctl --user enable --now btc-agent-v2.service
systemctl --user show btc-agent-v2.service -p MainPID -p NRestarts -p ActiveState -p SubState
