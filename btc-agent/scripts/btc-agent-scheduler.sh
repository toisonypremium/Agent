#!/data/data/com.termux/files/usr/bin/bash

set -u

APP_DIR="${BTC_AGENT_APP_DIR:-/data/data/com.termux/files/home/.openclaw/workspace/btc-agent}"
CONFIG_PATH="${BTC_AGENT_CONFIG:-config.yaml}"
LOG_DIR="${BTC_AGENT_LOG_DIR:-logs}"
MODE="${BTC_AGENT_MODE:-paper}"
LOCK_FILE="${BTC_AGENT_SCHEDULER_LOCK_FILE:-$HOME/.btc-agent-scheduler.lock}"
ENV_FILE="${BTC_AGENT_ENV_FILE:-$HOME/btc-agent.env}"

cd "$APP_DIR" || exit 1
mkdir -p "$LOG_DIR"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$LOG_DIR/scheduler-wrapper.log"
}

fail() {
  log "ERROR: $*"
  echo "ERROR: $*" >&2
  exit 1
}

# rotate_log FILE MAX_BYTES
# If FILE exceeds MAX_BYTES, move it to FILE.1 (overwriting any prior .1),
# then start a fresh FILE. Keeps exactly one backup.
rotate_log() {
  local f="$1" max="$2"
  [ -f "$f" ] || return 0
  local size
  size=$(wc -c < "$f" 2>/dev/null || echo 0)
  if [ "$size" -gt "$max" ]; then
    mv -f "$f" "${f}.1"
    : > "$f"
  fi
}

run_with_rotating_log() {
  local log_file="$1"
  shift
  "$@" 2>&1 | while IFS= read -r line; do
    rotate_log "$log_file" 5242880
    printf '%s\n' "$line" >> "$log_file"
  done
  local status=${PIPESTATUS[0]}
  return "$status"
}

if [ -f "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  log "loaded env file: $ENV_FILE"
fi

case "$MODE" in
  paper|live-proof|live-canary-auto) ;;
  *) fail "invalid BTC_AGENT_MODE=$MODE; use paper|live-proof|live-canary-auto" ;;
esac

if [ -f "$LOCK_FILE" ]; then
  oldpid="$(cat "$LOCK_FILE" 2>/dev/null || true)"
  if [ -n "$oldpid" ] && kill -0 "$oldpid" 2>/dev/null; then
    echo "btc-agent scheduler already running pid=$oldpid"
    exit 0
  fi
fi

echo "$$" > "$LOCK_FILE"
export BTC_AGENT_SCHEDULER_LOCK_HELD=true
trap 'rm -f "$LOCK_FILE"' EXIT INT TERM

if [ ! -x ./bin/btc-agent ]; then
  fail "missing ./bin/btc-agent; run: go build -o bin/btc-agent ."
fi

log "scheduler wrapper start mode=$MODE config=$CONFIG_PATH pid=$$"

case "$MODE" in
  paper|live-proof)
    log "starting scheduler dry-run mode=$MODE"
    rotate_log "$LOG_DIR/scheduler.log" 5242880
    rotate_log "$LOG_DIR/scheduler-wrapper.log" 1048576
    run_with_rotating_log "$LOG_DIR/scheduler.log" ./bin/btc-agent scheduler --config "$CONFIG_PATH" --run-now --dry-run
    ;;
  live-canary-auto)
    if [ "${BTC_AGENT_ALLOW_AUTO_LIVE:-}" != "true" ]; then
      fail "live-canary-auto requires BTC_AGENT_ALLOW_AUTO_LIVE=true"
    fi
    if ./bin/btc-agent live-doctor --config "$CONFIG_PATH" >> "$LOG_DIR/live-doctor.log" 2>&1; then
      if grep -q 'Status: DOCTOR_BLOCK' reports/live_doctor_latest.md 2>/dev/null; then
        fail "live doctor blocked runtime; see reports/live_doctor_latest.md"
      fi
      log "live doctor passed or warned; starting real scheduler"
      rotate_log "$LOG_DIR/scheduler.log" 5242880
      rotate_log "$LOG_DIR/scheduler-wrapper.log" 1048576
      run_with_rotating_log "$LOG_DIR/scheduler.log" ./bin/btc-agent scheduler --config "$CONFIG_PATH" --run-now
    fi
    fail "live doctor command failed; see $LOG_DIR/live-doctor.log"
    ;;
esac
