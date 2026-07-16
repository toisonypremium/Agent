#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

ENV_FILE="/home/admin/btc-agent-systemd.env"
if [ ! -r "$ENV_FILE" ]; then
  echo "missing required env file: $ENV_FILE" >&2
  exit 1
fi
# Match the systemd service environment for all operational commands.
set -a
# shellcheck disable=SC1091
source "$ENV_FILE"
set +a

exec ./bin/btc-agent "$@"
