#!/usr/bin/env bash
set -euo pipefail
script="$(dirname "$0")/verify-immutable-runtime.sh"
bash -n "$script"
for expected in 'btc-agent-immutable-observe.timer' 'btc-agent-immutable-backup.timer' 'mode=paper paper_trading=true real_trading_enabled=false' 'Operator halt: ACTIVE' 'pragma quick_check' 'verify-backup.sh' 'systemctl --user --failed'; do
  grep -Fq "$expected" "$script" || { echo "missing runtime verification gate: $expected" >&2; exit 1; }
done
printf 'immutable_runtime_verify_drill=PASS\n'
