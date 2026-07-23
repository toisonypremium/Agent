#!/usr/bin/env bash
set -euo pipefail
root="$(mktemp -d "${PWD}/.release-test.XXXXXX")"
cleanup() { rm -rf "$root"; }
trap cleanup EXIT
mkdir -p "$root/input" "$root/runtime"
printf 'agent-test-binary' > "$root/input/agent"
sha="$(sha256sum "$root/input/agent" | awk '{print $1}')"
if BTC_AGENT_ROOT="$root" bash deploy/install-immutable-user-release.sh demo "$root/input/agent" "$sha" >/dev/null 2>&1; then echo 'approval gate missing' >&2; exit 1; fi
BTC_AGENT_RELEASE_APPROVED=yes BTC_AGENT_ROOT="$root" bash deploy/install-immutable-user-release.sh demo "$root/input/agent" "$sha" | grep -q PASS
test "$(readlink -f "$root/immutable/current")" = "$root/immutable/releases/demo"
test "$(sha256sum "$root/immutable/current/agent" | awk '{print $1}')" = "$sha"
test ! -e "$root/runtime/config.yaml"
if BTC_AGENT_RELEASE_APPROVED=yes BTC_AGENT_ROOT="$root" bash deploy/install-immutable-user-release.sh demo "$root/input/agent" "$sha" >/dev/null 2>&1; then echo 'release overwrite allowed' >&2; exit 1; fi
printf 'immutable_user_release_drill=PASS\n'
