#!/usr/bin/env bash
# Installs the only supported unprivileged runtime on hosts where the dedicated
# agent account lacks sudo. It never clears operator halt or enables execution.
set -euo pipefail

unit_dir="${HOME}/.config/systemd/user"
source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for unit in btc-agent-immutable.service btc-agent-immutable-observe.service btc-agent-immutable-observe.timer; do
  install -D -m 0600 "$source_dir/$unit" "$unit_dir/$unit"
done
systemctl --user daemon-reload
systemd-analyze --user verify "$unit_dir/btc-agent-immutable.service"
systemctl --user enable --now btc-agent-immutable.service btc-agent-immutable-observe.timer
systemctl --user is-active --quiet btc-agent-immutable.service
printf 'immutable_user_service_install=PASS\n'
