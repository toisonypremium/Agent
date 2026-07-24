#!/usr/bin/env bash
set -euo pipefail
root="${BTC_AGENT_ROOT:-$HOME/btc-agent}"
unit=btc-agent-web-console.service
tunnel_unit=btc-agent-web-console-tunnel.service
[[ "$(systemctl --user is-enabled "$unit")" == enabled ]]
[[ "$(systemctl --user is-active "$unit")" == active ]]
[[ "$(systemctl --user is-active "$tunnel_unit")" == active ]]
[[ "$(pgrep -fc "^$root/web-console/current/web-console$")" == 1 ]]
ss -ltn "sport = :8787" | grep -q '127.0.0.1:8787'
curl --fail --silent --show-error http://127.0.0.1:8787/healthz | grep -q '"status":"ok"'
curl --fail --silent --show-error http://127.0.0.1:8787/api/v1/overview | grep -q '"schema_version":1'
[[ "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:8787/.env)" == 404 ]]
[[ "$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:8787/api/v1/orders)" == 404 ]]
[[ "$(sha256sum "$root/web-console/current/web-console" | awk '{print $1}')" == "$(awk '{print $1}' "$root/web-console/current/web-console.sha256")" ]]
printf 'web_console_runtime_verify=PASS release=%s tunnel=active\n' "$(readlink -f "$root/web-console/current")"
