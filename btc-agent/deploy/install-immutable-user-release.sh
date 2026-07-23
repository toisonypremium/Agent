#!/usr/bin/env bash
# Installs an already built Linux/amd64 agent without altering runtime config,
# credentials, operator halt, or execution settings. Restart is opt-in.
set -euo pipefail

if [[ "${BTC_AGENT_RELEASE_APPROVED:-}" != "yes" ]]; then
  echo 'set BTC_AGENT_RELEASE_APPROVED=yes to install a release' >&2
  exit 64
fi
if [[ $# -ne 3 ]]; then
  echo "usage: $0 <release-id> <linux-amd64-agent> <expected-sha256>" >&2
  exit 64
fi
release_id="$1"
source_binary="$2"
expected="$3"
[[ "$release_id" =~ ^[A-Za-z0-9._-]+$ ]] || { echo 'invalid release id' >&2; exit 64; }
[[ -f "$source_binary" ]] || { echo 'source binary missing' >&2; exit 66; }
actual="$(sha256sum "$source_binary" | awk '{print $1}')"
[[ "$actual" == "$expected" ]] || { echo 'binary SHA-256 mismatch' >&2; exit 65; }
root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
releases="$root/immutable/releases"
mkdir -p "$releases"
release="$releases/$release_id"
[[ ! -e "$release" ]] || { echo 'release id already exists' >&2; exit 73; }
stage="$(mktemp -d "$releases/.${release_id}.XXXXXX")"
cleanup() { rm -rf "$stage"; }
trap cleanup EXIT
install -m 0550 "$source_binary" "$stage/agent"
printf '%s  agent\n' "$expected" > "$stage/agent.sha256"
printf 'release_id=%s\nbuild_sha256=%s\ninstalled_at=%s\n' "$release_id" "$expected" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$stage/manifest"
[[ "$(sha256sum "$stage/agent" | awk '{print $1}')" == "$expected" ]]
mv "$stage" "$release"
trap - EXIT
ln -sfn "$release" "$root/immutable/current.next"
mv -Tf "$root/immutable/current.next" "$root/immutable/current"
if [[ "${BTC_AGENT_RESTART_IMMUTABLE_SERVICE:-no}" == yes ]]; then
  systemctl --user restart btc-agent-immutable.service
fi
printf 'immutable_user_release_install=PASS release=%s sha256=%s restarted=%s\n' "$release_id" "$expected" "${BTC_AGENT_RESTART_IMMUTABLE_SERVICE:-no}"
