#!/usr/bin/env bash
set -euo pipefail

src="${AGENT_DATA_DIR:-/var/lib/agent}"
dst="${AGENT_BACKUP_DIR:-/var/backups/agent/snapshots}"
keep="${AGENT_BACKUP_RETENTION_DAYS:-14}"

command -v python3 >/dev/null || { echo 'python3 is required for SQLite-safe backups' >&2; exit 1; }
[[ -d "$src" ]] || { echo "data directory missing: $src" >&2; exit 1; }
mkdir -p "$dst"
chmod 0700 "$dst"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
stage="$(mktemp -d "$dst/.agent-$stamp.XXXXXX")"
archive_tmp="$dst/.agent-$stamp.tar.gz.tmp"
out="$dst/agent-$stamp.tar.gz"
cleanup() { rm -rf "$stage" "$archive_tmp"; }
trap cleanup EXIT

# Copy ordinary state first. SQLite databases are copied separately using the online
# backup API; copying a WAL-mode main file alone can silently lose recent writes.
tar --exclude='*.db' --exclude='*.db-wal' --exclude='*.db-shm' -C "$src" -cf - . | tar -C "$stage" -xf -
while IFS= read -r -d '' db; do
  relative="${db#"$src"/}"
  target="$stage/$relative"
  mkdir -p "$(dirname "$target")"
  python3 - "$db" "$target" <<'PY'
import sqlite3, sys
source, target = sys.argv[1:]
read = sqlite3.connect(f"file:{source}?mode=ro", uri=True)
write = sqlite3.connect(target)
try:
    read.backup(write)
    result = write.execute("PRAGMA quick_check").fetchone()[0]
    if result.lower() != "ok":
        raise SystemExit(f"SQLite quick_check failed for {source}: {result}")
finally:
    write.close()
    read.close()
PY
done < <(find "$src" -type f -name '*.db' -print0)

(
  cd "$stage"
  if find . -type f ! -name SHA256SUMS -print -quit | grep -q .; then
    find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -r -0 sha256sum > SHA256SUMS
  else
    : > SHA256SUMS
  fi
  printf 'created_at=%s\nsource=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$src" > MANIFEST
)
tar -C "$stage" -czf "$archive_tmp" .
python3 - "$archive_tmp" <<'PY'
import sys, tarfile
with tarfile.open(sys.argv[1], "r:gz") as archive:
    archive.getmember("./SHA256SUMS")
    archive.getmember("./MANIFEST")
PY
mv -f "$archive_tmp" "$out"
sha256sum "$out" > "$out.sha256"
find "$dst" -type f \( -name 'agent-*.tar.gz' -o -name 'agent-*.tar.gz.sha256' \) -mtime +"$keep" -delete
printf '%s\n' "$out"
