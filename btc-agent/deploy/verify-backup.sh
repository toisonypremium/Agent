#!/usr/bin/env bash
set -euo pipefail

archive="${1:?usage: verify-backup.sh /path/to/agent-<timestamp>.tar.gz}"
checksum="${archive}.sha256"

[[ -f "$archive" ]] || { echo "backup archive missing: $archive" >&2; exit 1; }
[[ -f "$checksum" ]] || { echo "backup checksum missing: $checksum" >&2; exit 1; }
command -v python3 >/dev/null || { echo 'python3 is required for SQLite verification' >&2; exit 1; }

sha256sum --check "$checksum"
stage="$(mktemp -d)"
cleanup() { rm -rf "$stage"; }
trap cleanup EXIT

tar -C "$stage" -xzf "$archive"
[[ -f "$stage/SHA256SUMS" ]] || { echo 'backup manifest SHA256SUMS missing' >&2; exit 1; }
[[ -f "$stage/MANIFEST" ]] || { echo 'backup manifest MANIFEST missing' >&2; exit 1; }
(
  cd "$stage"
  sha256sum --check SHA256SUMS
)

count=0
while IFS= read -r -d '' db; do
  python3 - "$db" <<'PY'
import sqlite3, sys
path = sys.argv[1]
conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
try:
    result = conn.execute("PRAGMA quick_check").fetchone()[0]
    if result.lower() != "ok":
        raise SystemExit(f"SQLite quick_check failed for {path}: {result}")
finally:
    conn.close()
PY
  count=$((count + 1))
done < <(find "$stage" -type f -name '*.db' -print0)
printf 'backup_verify=PASS sqlite_databases=%d archive=%s\n' "$count" "$archive"
