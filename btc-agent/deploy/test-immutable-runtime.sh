#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
for file in \
  deploy/immutable-observe.sh \
  deploy/immutable-backup.sh \
  deploy/verify-immutable-backup.sh \
  deploy/systemd/btc-agent-immutable.service \
  deploy/systemd/btc-agent-immutable-observe.service \
  deploy/systemd/btc-agent-immutable-observe.timer \
  deploy/systemd/btc-agent-immutable-backup.service \
  deploy/systemd/btc-agent-immutable-backup.timer \
  deploy/systemd/btc-agent-immutable-daily-check.service \
  deploy/systemd/btc-agent-immutable-daily-check.timer \
  deploy/check-runtime-daily.sh \
  deploy/systemd/install-immutable-user-service.sh; do
  test -f "$file" || { echo "missing immutable runtime artifact: $file" >&2; exit 1; }
done
bash -n deploy/immutable-observe.sh deploy/immutable-backup.sh deploy/verify-immutable-backup.sh deploy/systemd/install-immutable-user-service.sh
for expected in \
  'ExecStart=%h/btc-agent/immutable/current/agent scheduler --config %h/btc-agent/runtime/config.yaml' \
  'NoNewPrivileges=true' \
  'ProtectSystem=strict' \
  'ProtectHome=read-only' \
  'ExecStart=%h/btc-agent/immutable/immutable-observe.sh' \
  'ExecStart=%h/btc-agent/immutable/backup.sh' \
  'OnCalendar=daily' \
  'Persistent=true'; do
  grep -RFxq "$expected" deploy/systemd || { echo "missing immutable hardening directive: $expected" >&2; exit 1; }
done
grep -Fq 'verify-immutable-backup.sh' deploy/systemd/install-immutable-user-service.sh
grep -Fq 'btc-agent-immutable-backup.timer' deploy/systemd/install-immutable-user-service.sh
printf 'immutable_runtime_drill=PASS\n'
