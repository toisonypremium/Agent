#!/usr/bin/env bash
set -euo pipefail

root="$(mktemp -d)"
cleanup() { rm -rf "$root"; }
trap cleanup EXIT
mkdir -p "$root/data" "$root/backups"
printf 'runtime-state\n' > "$root/data/state.txt"
python3 - "$root/data/btc_agent.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute('PRAGMA journal_mode=WAL')
conn.execute('CREATE TABLE orders (id INTEGER PRIMARY KEY, client_id TEXT NOT NULL)')
conn.execute("INSERT INTO orders(client_id) VALUES ('backup-drill')")
conn.commit()
conn.close()
PY
archive="$(AGENT_DATA_DIR="$root/data" AGENT_BACKUP_DIR="$root/backups" AGENT_BACKUP_RETENTION_DAYS=1 bash "$(dirname "$0")/backup.sh")"
bash "$(dirname "$0")/verify-backup.sh" "$archive" | grep -q 'backup_verify=PASS sqlite_databases=1'
restore="$root/restore"
mkdir -p "$restore"
tar -C "$restore" -xzf "$archive"
python3 - "$restore/btc_agent.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
try:
    row = conn.execute('SELECT client_id FROM orders').fetchone()
    assert row == ('backup-drill',), row
finally:
    conn.close()
PY
printf 'backup_drill=PASS\n'
