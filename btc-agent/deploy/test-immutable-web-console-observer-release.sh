#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d)"; trap 'rm -rf "$root"' EXIT
artifact="$root/observer"; printf '#!/bin/sh
exit 0
' > "$artifact"; chmod 0755 "$artifact"
sha="$(sha256sum "$artifact" | awk '{print $1}')"
HOME="$root/home" BTC_AGENT_WEB_OBSERVER_RELEASE_APPROVED=yes bash deploy/install-web-console-observer-user-release.sh web-observer-abcdef0-20260724T030411Z "$artifact" "$sha" >/dev/null
test -x "$root/home/btc-agent/web-console-observer/current/web-console-observer"
test "$(cat "$root/home/btc-agent/web-console-observer/current/web-console-observer.sha256")" = "$sha"
if HOME="$root/other" bash deploy/install-web-console-observer-user-release.sh web-observer-abcdef0-20260724T030411Z "$artifact" "$sha" >/dev/null 2>&1; then exit 1; fi
printf 'immutable_web_console_observer_release_drill=PASS\n'
