#!/usr/bin/env bash
set -euo pipefail

root="$(mktemp -d "${PWD}/.immutable-backup-test.XXXXXX")"
cleanup() { rm -rf "$root"; }
trap cleanup EXIT
mkdir -p "$root/runtime/data" "$root/immutable/releases/test"
printf 'runtime-state\n' > "$root/runtime/state.txt"
ln -s "$root/immutable/releases/test" "$root/immutable/current"
python3 - "$root/runtime/data/btc_agent.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute('PRAGMA journal_mode=WAL')
conn.execute('CREATE TABLE orders (id INTEGER PRIMARY KEY, client_id TEXT NOT NULL)')
conn.execute("INSERT INTO orders(client_id) VALUES ('immutable-backup-drill')")
conn.commit()
conn.close()
PY
archive="$(BTC_AGENT_ROOT="$root" BTC_AGENT_BACKUP_RETENTION_DAYS=1 bash deploy/immutable-backup.sh)"
BTC_AGENT_ROOT="$root" bash deploy/verify-immutable-backup.sh "$archive" | grep -q 'immutable_backup_verify=PASS'
restore="$root/restore"
mkdir -p "$restore"
tar -C "$restore" -xzf "$archive"
python3 - "$restore/data/btc_agent.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
try:
    assert conn.execute('SELECT client_id FROM orders').fetchone() == ('immutable-backup-drill',)
finally:
    conn.close()
PY
test -f "$restore/SHA256SUMS"
test -f "$restore/MANIFEST"
printf 'immutable_backup_drill=PASS\n'
