#!/usr/bin/env bash
# Creates an online SQLite-consistent runtime backup without stopping the scheduler.
set -euo pipefail

root="${BTC_AGENT_ROOT:-${HOME}/btc-agent}"
src="$root/runtime"
out="${BTC_AGENT_BACKUP_DIR:-$src/backups}"
keep="${BTC_AGENT_BACKUP_RETENTION_DAYS:-14}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out"
chmod 0700 "$out"
stage="$(mktemp -d "$out/.snapshot-$stamp.XXXXXX")"
archive_tmp="$out/.snapshot-$stamp.tar.gz.tmp"
archive="$out/snapshot-$stamp.tar.gz"
cleanup() { rm -rf "$stage" "$archive_tmp"; }
trap cleanup EXIT

tar --exclude='./data/*.db' --exclude='./data/*.db-wal' --exclude='./data/*.db-shm' --exclude='./backups' -C "$src" -cf - . | tar -C "$stage" -xf -
python3 - "$src/data/btc_agent.db" "$stage/data/btc_agent.db" <<'PY'
import sqlite3, sys
read = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
write = sqlite3.connect(sys.argv[2])
try:
    read.backup(write)
    assert write.execute("PRAGMA quick_check").fetchone()[0].lower() == "ok"
finally:
    write.close()
    read.close()
PY
(
  cd "$stage"
  find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
  printf 'created_at=%s\nrelease=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(readlink -f "$root/immutable/current")" > MANIFEST
)
tar -C "$stage" -czf "$archive_tmp" .
mv "$archive_tmp" "$archive"
sha256sum "$archive" > "$archive.sha256"
find "$out" -type f \( -name 'snapshot-*.tar.gz' -o -name 'snapshot-*.tar.gz.sha256' \) -mtime +"$keep" -delete
printf '%s\n' "$archive"
