#!/data/data/com.termux/files/usr/bin/bash

set -u

APP_DIR="${BTC_AGENT_APP_DIR:-/data/data/com.termux/files/home/.openclaw/workspace/btc-agent}"
LOG_DIR="${BTC_AGENT_LOG_DIR:-logs}"
ENV_FILE="${BTC_AGENT_ENV_FILE:-$HOME/btc-agent.env}"
LOCK_FILE="${BTC_AGENT_SCHEDULER_LOCK_FILE:-$HOME/.btc-agent-scheduler.lock}"

cd "$APP_DIR" || exit 1
mkdir -p "$LOG_DIR"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$LOG_DIR/scheduler-wrapper.log"
}

if [ -f "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  log "loaded env file: $ENV_FILE"
fi

log "restart requested"

old_pids="$(pgrep -f './bin/btc-agent scheduler --config' 2>/dev/null || true)"
if [ -n "$old_pids" ]; then
  log "stopping scheduler pids: $old_pids"
  kill $old_pids 2>/dev/null || true
  sleep 2
fi

leftover_pids="$(pgrep -f './bin/btc-agent scheduler --config' 2>/dev/null || true)"
if [ -n "$leftover_pids" ]; then
  log "force stopping scheduler pids: $leftover_pids"
  kill -9 $leftover_pids 2>/dev/null || true
  sleep 1
fi

rm -f "$LOCK_FILE"

if [ ! -x ./scripts/btc-agent-scheduler.sh ]; then
  log "ERROR: missing ./scripts/btc-agent-scheduler.sh"
  echo "ERROR: missing ./scripts/btc-agent-scheduler.sh" >&2
  exit 1
fi

# Important: append, never overwrite. Scheduler stdout/stderr lands in scheduler.log
# through btc-agent-scheduler.sh; wrapper lifecycle logs land in scheduler-wrapper.log.
nohup ./scripts/btc-agent-scheduler.sh >> "$LOG_DIR/scheduler-wrapper.log" 2>&1 &
new_pid="$!"
log "restart launched wrapper pid=$new_pid"
echo "scheduler wrapper pid=$new_pid"
