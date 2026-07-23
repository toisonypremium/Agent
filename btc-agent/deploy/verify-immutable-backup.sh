#!/usr/bin/env bash
# Validates checksum, manifest checksums, and SQLite integrity of one immutable backup.
set -euo pipefail
archive="${1:?usage: verify-immutable-backup.sh /path/to/snapshot-<timestamp>.tar.gz}"
sha256sum --check "$archive.sha256"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
tar -C "$stage" -xzf "$archive"
(cd "$stage"; sha256sum --check SHA256SUMS)
python3 - "$stage/data/btc_agent.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
try:
    assert conn.execute("PRAGMA quick_check").fetchone()[0].lower() == "ok"
finally:
    conn.close()
PY
printf 'immutable_backup_verify=PASS archive=%s\n' "$archive"
