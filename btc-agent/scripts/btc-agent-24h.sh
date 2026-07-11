#!/data/data/com.termux/files/usr/bin/bash

set -u

APP_DIR="/data/data/com.termux/files/home/.openclaw/workspace/btc-agent"
CONFIG_PATH="${BTC_AGENT_CONFIG:-config.yaml}"
LOG_DIR="${BTC_AGENT_LOG_DIR:-logs}"
BACKUP_DIR="${BTC_AGENT_BACKUP_DIR:-backups}"
LOCK_FILE="${BTC_AGENT_LOCK_FILE:-$HOME/.btc-agent-24h.lock}"
RETENTION_DAYS="${BTC_AGENT_BACKUP_RETENTION_DAYS:-14}"
SLEEP_SECONDS="${BTC_AGENT_LOOP_SLEEP_SECONDS:-60}"
DAILY_HOUR="${BTC_AGENT_DAILY_HOUR:-08}"
DAILY_WINDOW_MINUTES="${BTC_AGENT_DAILY_WINDOW_MINUTES:-10}"
AI_WATCH_HOURS="${BTC_AGENT_AI_WATCH_HOURS:-00 04 08 12 16 20}"
LIVE_PROOF_HOURS="${BTC_AGENT_LIVE_PROOF_HOURS:-01 07 13 19}"
MODE="${BTC_AGENT_MODE:-paper}"

cd "$APP_DIR" || exit 1
mkdir -p "$LOG_DIR" "$BACKUP_DIR"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$LOG_DIR/agent-24h.log"
}

fail() {
  log "ERROR: $*"
  exit 1
}

case "$MODE" in
  paper|live-proof|live-auto) ;;
  *) fail "invalid BTC_AGENT_MODE=$MODE; use paper|live-proof|live-auto" ;;
esac

if [ -f "$LOCK_FILE" ]; then
  oldpid="$(cat "$LOCK_FILE" 2>/dev/null || true)"
  if [ -n "$oldpid" ] && kill -0 "$oldpid" 2>/dev/null; then
    echo "btc-agent 24/7 loop already running pid=$oldpid"
    exit 0
  fi
fi

echo "$$" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT INT TERM

require_binary() {
  if [ ! -x ./bin/btc-agent ]; then
    fail "missing ./bin/btc-agent; run: go build -o bin/btc-agent ."
  fi
}

allow_live_command() {
  cmd="$1"
  case "$cmd" in
    execute-live-proof-order)
      return 1
      ;;
    auto-live-order)
      [ "$MODE" = "live-auto" ] && [ "${BTC_AGENT_ALLOW_AUTO_LIVE:-}" = "true" ]
      return $?
      ;;
    live-proof|live-readiness|reconcile-live-orders|live-positions|operator-status)
      [ "$MODE" = "live-proof" ] || [ "$MODE" = "live-auto" ]
      return $?
      ;;
  esac
  return 0
}

run_btc_agent() {
  cmd="$1"
  shift
  if ! allow_live_command "$cmd"; then
    log "blocked command in mode=$MODE: $cmd"
    return 1
  fi
  ./bin/btc-agent "$cmd" --config "$CONFIG_PATH" "$@"
}

backup_db() {
  db_path="${BTC_AGENT_DB_PATH:-data/btc_agent.db}"
  if command -v python3 >/dev/null 2>&1; then
    parsed="$(python3 - "$CONFIG_PATH" <<'PY' 2>/dev/null || true
import sys
path = 'data/btc_agent.db'
in_storage = False
for raw in open(sys.argv[1], encoding='utf-8'):
    stripped = raw.strip()
    if not stripped or stripped.startswith('#'):
        continue
    if not raw.startswith((' ', '\t')):
        in_storage = stripped == 'storage:'
        continue
    if in_storage and stripped.startswith('path:'):
        path = stripped.split(':', 1)[1].strip().strip('"\'')
        break
print(path)
PY
)"
    if [ -n "$parsed" ]; then
      db_path="$parsed"
    fi
  fi
  if [ ! -f "$db_path" ]; then
    log "backup skipped: db not found at $db_path"
    return 0
  fi
  stamp="$(date '+%Y-%m-%d_%H%M%S')"
  cp "$db_path" "$BACKUP_DIR/btc_agent-$stamp.db"
  find "$BACKUP_DIR" -name 'btc_agent-*.db' -type f -mtime +"$RETENTION_DAYS" -delete 2>/dev/null || true
  log "backup ok: $BACKUP_DIR/btc_agent-$stamp.db"
}

minute_in_daily_window() {
  minute="$1"
  [ "$minute" -lt "$DAILY_WINDOW_MINUTES" ] 2>/dev/null
}

hour_in_schedule() {
  hour="$1"
  shift
  for h in "$@"; do
    if [ "$hour" = "$h" ]; then
      return 0
    fi
  done
  return 1
}

run_live_readiness_stack() {
  log "live-readiness start mode=$MODE"
  run_btc_agent live-readiness >> "$LOG_DIR/live-readiness.log" 2>&1 || log "live-readiness failed"
  run_btc_agent live-proof >> "$LOG_DIR/live-proof.log" 2>&1 || log "live-proof failed"
  run_btc_agent reconcile-live-orders >> "$LOG_DIR/live-reconcile.log" 2>&1 || log "live reconcile failed"
  run_btc_agent live-positions >> "$LOG_DIR/live-positions.log" 2>&1 || log "live positions failed"
}

run_live_auto_attempt() {
  if [ "$MODE" != "live-auto" ]; then
    return 0
  fi
  if [ "${BTC_AGENT_ALLOW_AUTO_LIVE:-}" != "true" ]; then
    log "auto live skipped: BTC_AGENT_ALLOW_AUTO_LIVE=true not set"
    return 0
  fi
  log "auto-live-order start live auto mode"
  if run_btc_agent auto-live-order >> "$LOG_DIR/auto-live-order.log" 2>&1; then
    log "auto-live-order finished"
  else
    log "auto-live-order blocked/failed; see $LOG_DIR/auto-live-order.log"
  fi
  run_btc_agent reconcile-live-orders >> "$LOG_DIR/live-reconcile.log" 2>&1 || log "post-auto reconcile failed"
}

require_binary
log "btc-agent 24/7 loop started config=$CONFIG_PATH mode=$MODE pid=$$"

while true; do
  hour="$(date '+%H')"
  minute="$(date '+%M')"
  today="$(date '+%F')"

  log "heartbeat mode=$MODE"

  case "$minute" in
    00|15|30|45)
      marker="$LOG_DIR/.auto-live-$today-$hour-$minute"
      if [ ! -f "$marker" ]; then
        run_live_auto_attempt
        touch "$marker"
      fi
      ;;
  esac

  if [ "$hour" = "$DAILY_HOUR" ] && minute_in_daily_window "$minute"; then
    marker="$LOG_DIR/.run-daily-$today"
    if [ ! -f "$marker" ]; then
      log "run-daily start"
      if run_btc_agent run-daily >> "$LOG_DIR/run-daily.log" 2>&1; then
        log "run-daily ok"
        touch "$marker"
      else
        log "run-daily failed; see $LOG_DIR/run-daily.log"
      fi

      log "maintenance start"
      if run_btc_agent maintenance >> "$LOG_DIR/maintenance.log" 2>&1; then
        log "maintenance ok"
      else
        log "maintenance failed; see $LOG_DIR/maintenance.log"
      fi

      backup_db >> "$LOG_DIR/backup.log" 2>&1
    fi
  fi

  if [ "$minute" = "15" ] && hour_in_schedule "$hour" $AI_WATCH_HOURS; then
    marker="$LOG_DIR/.ai-watch-$today-$hour"
    if [ ! -f "$marker" ]; then
      log "run-ai-watch start hour=$hour"
      if run_btc_agent run-ai-watch >> "$LOG_DIR/ai-watch.log" 2>&1; then
        log "run-ai-watch ok hour=$hour"
        touch "$marker"
      else
        log "run-ai-watch failed hour=$hour; see $LOG_DIR/ai-watch.log"
      fi
    fi
  fi

  if [ "$minute" = "25" ] && hour_in_schedule "$hour" $LIVE_PROOF_HOURS; then
    marker="$LOG_DIR/.live-proof-$today-$hour"
    if [ ! -f "$marker" ]; then
      run_live_readiness_stack
      touch "$marker"
    fi
  fi

  if [ "$minute" = "45" ]; then
    marker="$LOG_DIR/.status-$today-$hour"
    if [ ! -f "$marker" ]; then
      if run_btc_agent status >> "$LOG_DIR/status.log" 2>&1; then
        log "status ok hour=$hour"
        touch "$marker"
      else
        log "status failed hour=$hour; see $LOG_DIR/status.log"
      fi
    fi
  fi

  sleep "$SLEEP_SECONDS"
done
