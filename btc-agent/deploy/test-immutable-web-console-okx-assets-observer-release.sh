#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d)"; trap 'rm -rf "$root"' EXIT
artifact="$root/observer"; printf '#!/bin/sh\nexit 0\n' > "$artifact"; chmod 0755 "$artifact"
sha="$(sha256sum "$artifact" | awk '{print $1}')"
HOME="$root/home" BTC_AGENT_OKX_ASSETS_OBSERVER_RELEASE_APPROVED=yes bash deploy/install-web-console-okx-assets-observer-user-release.sh okx-assets-observer-abcdef0-20260724T033000Z "$artifact" "$sha" >/dev/null
test -x "$root/home/btc-agent/web-console-okx-assets-observer/current/web-console-okx-assets-observer"
test "$(cat "$root/home/btc-agent/web-console-okx-assets-observer/current/web-console-okx-assets-observer.sha256")" = "$sha"
if HOME="$root/other" bash deploy/install-web-console-okx-assets-observer-user-release.sh okx-assets-observer-abcdef0-20260724T033000Z "$artifact" "$sha" >/dev/null 2>&1; then exit 1; fi
printf 'immutable_okx_assets_observer_release_drill=PASS\n'
