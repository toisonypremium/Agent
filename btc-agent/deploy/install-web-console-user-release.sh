#!/usr/bin/env bash
# Installs a separate, loopback-only Web Console release. It never changes the
# scheduler service, runtime config, database, or operator halt state.
set -euo pipefail
[[ "${BTC_AGENT_WEB_RELEASE_APPROVED:-}" == yes ]] || { echo 'set BTC_AGENT_WEB_RELEASE_APPROVED=yes' >&2; exit 64; }
[[ $# -eq 4 ]] || { echo "usage: $0 <release-id> <web-console-linux-amd64> <static-dir> <expected-sha256>" >&2; exit 64; }
release_id="$1"; binary="$2"; static="$3"; expected="$4"
[[ "$release_id" =~ ^[A-Za-z0-9._-]+$ ]] || { echo 'invalid release id' >&2; exit 64; }
[[ -f "$binary" && -d "$static" ]] || { echo 'release input missing' >&2; exit 66; }
actual="$(sha256sum "$binary" | awk '{print $1}')"
[[ "$actual" == "$expected" ]] || { echo 'binary SHA-256 mismatch' >&2; exit 65; }
root="${BTC_AGENT_ROOT:-$HOME/btc-agent}"
releases="$root/web-console/releases"; release="$releases/$release_id"
mkdir -p "$releases"; [[ ! -e "$release" ]] || { echo 'release exists' >&2; exit 73; }
stage="$(mktemp -d "$releases/.${release_id}.XXXXXX")"; trap 'rm -rf "$stage"' EXIT
install -m 0550 "$binary" "$stage/web-console"
cp -a "$static" "$stage/static"; find "$stage/static" -type d -exec chmod 0550 {} +; find "$stage/static" -type f -exec chmod 0440 {} +
printf '%s  web-console\n' "$expected" > "$stage/web-console.sha256"
printf 'release_id=%s\nbuild_sha256=%s\ninstalled_at=%s\n' "$release_id" "$expected" "$(date -u +%FT%TZ)" > "$stage/manifest"
mv "$stage" "$release"; trap - EXIT
ln -sfn "$release" "$root/web-console/current.next"; mv -Tf "$root/web-console/current.next" "$root/web-console/current"
printf 'web_console_release_install=PASS release=%s sha256=%s\n' "$release_id" "$expected"
