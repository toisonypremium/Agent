#!/bin/bash
set -euo pipefail
umask 077
ROOT=/home/admin/btc-agent/btc-agent
SIDE=/home/admin/circuit-framework-staging
OUT="$ROOT/reports/circuit"
SHA=7ab0137caef9e09b5666c8fbb1e10353aa4eb4b3
mkdir -p "$OUT"
input=$(mktemp "$OUT/.input.XXXXXX")
candidate=$(mktemp "$OUT/.evidence.XXXXXX")
trap 'rm -f "$input" "$candidate"' EXIT
"$ROOT/bin/btc-agent-circuit" circuit-research-snapshot --config "$ROOT/config.yaml" >"$input"
/usr/bin/timeout 60 "$SIDE/.venv/bin/python" "$SIDE/adapter/run_deterministic.py" --input "$input" --output "$candidate" --producer-commit "$SHA"
"$ROOT/bin/btc-agent-circuit" circuit-research-validate --config "$ROOT/config.yaml" --input "$input" --evidence "$candidate" --producer-commit "$SHA" >/dev/null
run_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["run_id"])' "$candidate")
safe_run_id=${run_id//[^A-Za-z0-9._-]/_}
runs="$OUT/runs"
mkdir -p "$runs"
install -m 0600 "$input" "$runs/${safe_run_id}.input.json"
install -m 0600 "$candidate" "$runs/${safe_run_id}.evidence.json"
install -m 0600 "$input" "$OUT/input_latest.json"
install -m 0600 "$candidate" "$OUT/evidence_latest.json"
find "$runs" -type f -mtime +30 -delete
printf 'CIRCUIT_RUN_OK run_id=%s producer=%s\n' "$safe_run_id" "$SHA"
