#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d)"
cleanup() { chmod -R u+w "$root" 2>/dev/null || true; rm -rf "$root"; }
trap cleanup EXIT
source_bin="$root/web-console"; static="$root/static"; mkdir -p "$static/assets"
printf 'fixture-binary' > "$source_bin"; printf '<!doctype html>' > "$static/index.html"; printf 'asset' > "$static/assets/app.js"
sha="$(sha256sum "$source_bin" | awk '{print $1}')"
if BTC_AGENT_ROOT="$root/runtime" bash "$(dirname "$0")/install-web-console-user-release.sh" r1 "$source_bin" "$static" "$sha" >/dev/null 2>&1; then
  echo 'approval gate missing' >&2; exit 1
fi
BTC_AGENT_ROOT="$root/runtime" BTC_AGENT_WEB_RELEASE_APPROVED=yes bash "$(dirname "$0")/install-web-console-user-release.sh" r1 "$source_bin" "$static" "$sha" | grep -q 'web_console_release_install=PASS'
[[ "$(sha256sum "$root/runtime/web-console/current/web-console" | awk '{print $1}')" == "$sha" ]]
[[ -f "$root/runtime/web-console/current/static/index.html" ]]
[[ -f "$root/runtime/web-console/current/static/assets/app.js" ]]
[[ ! -e "$root/runtime/immutable" ]]
printf 'web_console_release_drill=PASS\n'
