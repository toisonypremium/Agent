#!/usr/bin/env bash
set -euo pipefail
if [[ "${BTC_AGENT_WEB_OBSERVER_RELEASE_APPROVED:-}" != yes ]]; then
  echo "BTC_AGENT_WEB_OBSERVER_RELEASE_APPROVED=yes required" >&2
  exit 64
fi
if [[ $# -ne 3 ]]; then
  echo "usage: $0 <release-id> <observer-linux-amd64> <expected-sha256>" >&2
  exit 64
fi
release="$1"; artifact="$2"; expected="$3"
[[ "$release" =~ ^web-observer-[a-f0-9]{7,40}-[0-9]{8}T[0-9]{6}Z$ ]] || { echo "invalid release id" >&2; exit 64; }
[[ -f "$artifact" ]] || { echo "artifact missing" >&2; exit 66; }
actual="$(sha256sum "$artifact" | awk '{print $1}')"
[[ "$actual" == "$expected" ]] || { echo "sha256 mismatch" >&2; exit 65; }
root="$HOME/btc-agent/web-console-observer"; releases="$root/releases"; stage="$releases/.${release}.stage.$$"
mkdir -p "$releases"; trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage"; install -m 0550 "$artifact" "$stage/web-console-observer"
printf '%s\n' "$actual" > "$stage/web-console-observer.sha256"; chmod 0440 "$stage/web-console-observer.sha256"
mv "$stage" "$releases/$release"; ln -sfn "$releases/$release" "$root/.next"; mv -Tf "$root/.next" "$root/current"; trap - EXIT
printf 'web_console_observer_release_install=PASS release=%s sha256=%s\n' "$release" "$actual"
