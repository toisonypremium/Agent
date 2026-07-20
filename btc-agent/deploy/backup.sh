#!/usr/bin/env bash
set -euo pipefail
src="${AGENT_DATA_DIR:-/var/lib/agent}"; dst="${AGENT_BACKUP_DIR:-/var/backups/agent/snapshots}"
mkdir -p "$dst"; stamp="$(date -u +%Y%m%dT%H%M%SZ)"; out="$dst/agent-$stamp.tar.gz"
tar --exclude='*.db-shm' --exclude='*.db-wal' -czf "$out" -C "$src" .
echo "$out"
