#!/usr/bin/env bash
# Install an already-reviewed, immutable binary. This script intentionally never
# changes config, credentials, operator halt, or execution authority.
set -euo pipefail

release_id="${1:?usage: install-release.sh <git-sha-or-release-id> <binary-path>}"
binary="${2:?usage: install-release.sh <git-sha-or-release-id> <binary-path>}"
root="${AGENT_RELEASE_ROOT:-/opt/agent}"
releases="$root/releases"
target="$releases/$release_id"
service="${AGENT_SERVICE_NAME:-agent.service}"

[[ "$release_id" =~ ^[A-Za-z0-9._-]+$ ]] || { echo 'invalid release id' >&2; exit 64; }
[[ -x "$binary" ]] || { echo "release binary not executable: $binary" >&2; exit 1; }
[[ -n "${AGENT_RELEASE_APPROVED:-}" ]] || { echo 'AGENT_RELEASE_APPROVED is required' >&2; exit 1; }
[[ ! -e "$target" ]] || { echo "release already exists: $target" >&2; exit 1; }
command -v systemctl >/dev/null || { echo 'systemctl is required' >&2; exit 1; }

install -d -m 0750 "$releases"
stage="$(mktemp -d "$releases/.${release_id}.XXXXXX")"
cleanup() { rm -rf "$stage"; }
trap cleanup EXIT
install -m 0750 "$binary" "$stage/agent"
sha256sum "$stage/agent" > "$stage/agent.sha256"
printf 'release_id=%s\ninstalled_at=%s\n' "$release_id" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$stage/manifest"
"$stage/agent" version > "$stage/version.txt"
chmod 0550 "$stage/agent"
mv "$stage" "$target"
trap - EXIT
ln -s "$target" "$root/current.next"
mv -Tf "$root/current.next" "$root/current"
systemctl restart "$service"
systemctl is-active --quiet "$service"
"$(dirname "$0")/health-check.sh"
printf 'release_install=PASS release=%s sha256=%s\n' "$release_id" "$(cut -d' ' -f1 "$target/agent.sha256")"
