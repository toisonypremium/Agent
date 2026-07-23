#!/usr/bin/env bash
# Installs the only supported unprivileged runtime on hosts where the dedicated
# agent account lacks sudo. It never clears operator halt or enables execution.
set -euo pipefail

unit_dir="${HOME}/.config/systemd/user"
source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "$source_dir/../.." && pwd)"
runtime_dir="${BTC_AGENT_ROOT:-${HOME}/btc-agent}/immutable"
install -D -m 0700 "$project_dir/deploy/immutable-observe.sh" "$runtime_dir/immutable-observe.sh"
install -D -m 0700 "$project_dir/deploy/immutable-health-check.sh" "$runtime_dir/health-check.sh"
install -D -m 0700 "$project_dir/deploy/immutable-backup.sh" "$runtime_dir/backup.sh"
install -D -m 0700 "$project_dir/deploy/verify-immutable-backup.sh" "$runtime_dir/verify-backup.sh"
install -D -m 0700 "$project_dir/deploy/verify-immutable-runtime.sh" "$runtime_dir/verify-runtime.sh"
install -D -m 0700 "$project_dir/deploy/check-runtime-daily.sh" "$runtime_dir/check-runtime-daily.sh"
for unit in btc-agent-immutable.service btc-agent-immutable-observe.service btc-agent-immutable-observe.timer btc-agent-immutable-backup.service btc-agent-immutable-backup.timer btc-agent-immutable-daily-check.service btc-agent-immutable-daily-check.timer; do
  install -D -m 0600 "$source_dir/$unit" "$unit_dir/$unit"
done
systemctl --user daemon-reload
systemd-analyze --user verify "$unit_dir/btc-agent-immutable.service"
systemctl --user enable --now btc-agent-immutable.service btc-agent-immutable-observe.timer btc-agent-immutable-backup.timer btc-agent-immutable-daily-check.timer
systemctl --user is-active --quiet btc-agent-immutable.service btc-agent-immutable-observe.timer btc-agent-immutable-backup.timer btc-agent-immutable-daily-check.timer
printf 'immutable_user_service_install=PASS\n'
