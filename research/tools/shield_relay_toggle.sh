#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
STATE_DIR=${STATE_DIR:-/tmp/shield-relay-control}
PID_FILE="$STATE_DIR/tcp-relay.pid"
STATE_FILE="$STATE_DIR/state.env"
STDOUT_LOG="$STATE_DIR/tcp-relay.stdout"
STDERR_LOG="$STATE_DIR/tcp-relay.stderr"
RELAY_SCRIPT="$SCRIPT_DIR/tcp_relay.py"

command_name=${0##*/}
command=${1:-toggle}
shift || true

listen_host="0.0.0.0"
target_host="192.168.1.16"
allow_from="192.168.1.0/24"
ports_csv="6466,6467,8987"
log_dir="/tmp/shield-relay"

usage() {
  cat <<EOF
Usage:
  $command_name [on|off|toggle|status] [options]

Options:
  --listen-host HOST   Relay bind host (default: $listen_host)
  --target-host HOST   Real SHIELD host (default: $target_host)
  --allow-from CIDR    Firewall source allowlist (default: $allow_from)
  --ports CSV          Ports to relay/open (default: $ports_csv)
  --log-dir DIR        Relay session log directory (default: $log_dir)
  -h, --help           Show this help

Examples:
  $command_name on --target-host 192.168.1.16 --allow-from 192.168.1.42
  $command_name off
  $command_name status
EOF
}

log() {
  printf '[shield-relay] %s\n' "$*"
}

die() {
  printf '[shield-relay] ERROR: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --listen-host)
        listen_host=${2:-}
        shift 2
        ;;
      --target-host)
        target_host=${2:-}
        shift 2
        ;;
      --allow-from)
        allow_from=${2:-}
        shift 2
        ;;
      --ports)
        ports_csv=${2:-}
        shift 2
        ;;
      --log-dir)
        log_dir=${2:-}
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "unknown argument: $1"
        ;;
    esac
  done
}

normalize_ports() {
  local raw port
  IFS=',' read -r -a raw <<< "$ports_csv"
  ports=()
  for port in "${raw[@]}"; do
    port=${port//[[:space:]]/}
    [[ -n "$port" ]] || continue
    [[ "$port" =~ ^[0-9]+$ ]] || die "invalid port: $port"
    ports+=("$port")
  done
  ((${#ports[@]} > 0)) || die 'no valid ports specified'
}

save_state() {
  mkdir -p "$STATE_DIR"
  {
    printf 'listen_host=%q\n' "$listen_host"
    printf 'target_host=%q\n' "$target_host"
    printf 'allow_from=%q\n' "$allow_from"
    printf 'ports_csv=%q\n' "$ports_csv"
    printf 'log_dir=%q\n' "$log_dir"
  } > "$STATE_FILE"
}

load_state_if_present() {
  if [[ -f "$STATE_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$STATE_FILE"
  fi
}

relay_pid() {
  [[ -f "$PID_FILE" ]] || return 1
  local pid
  pid=$(<"$PID_FILE")
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  printf '%s\n' "$pid"
}

is_running() {
  local pid
  pid=$(relay_pid) || return 1
  kill -0 "$pid" 2>/dev/null
}

cleanup_stale_pid() {
  if [[ -f "$PID_FILE" ]] && ! is_running; then
    rm -f "$PID_FILE"
  fi
}

ufw_is_active() {
  local status
  status=$(sudo ufw status | sed -n '1p' | tr '[:upper:]' '[:lower:]')
  [[ "$status" == status:\ active* ]]
}

ufw_rule_exists() {
  local port=$1
  sudo ufw status | grep -Fq "$port/tcp" && sudo ufw status | grep -Fq "$allow_from"
}

open_ports() {
  need_cmd sudo
  need_cmd ufw
  if ! ufw_is_active; then
    log 'ufw is inactive; skipping firewall rule changes'
    return 0
  fi

  local port
  for port in "${ports[@]}"; do
    if ufw_rule_exists "$port"; then
      log "ufw rule already present for $allow_from -> tcp/$port"
      continue
    fi
    log "opening ufw for $allow_from -> tcp/$port"
    sudo ufw allow proto tcp from "$allow_from" to any port "$port" comment 'shield-relay'
  done
}

close_ports() {
  need_cmd sudo
  need_cmd ufw
  if ! ufw_is_active; then
    log 'ufw is inactive; nothing to close'
    return 0
  fi

  local port
  for port in "${ports[@]}"; do
    log "closing ufw for $allow_from -> tcp/$port"
    sudo ufw --force delete allow proto tcp from "$allow_from" to any port "$port" >/dev/null 2>&1 || true
  done
}

start_relay() {
  need_cmd python3
  [[ -f "$RELAY_SCRIPT" ]] || die "relay script not found: $RELAY_SCRIPT"
  mkdir -p "$STATE_DIR" "$log_dir"

  if is_running; then
    log "relay already running with pid $(relay_pid)"
    return 0
  fi

  save_state
  log "starting relay -> $target_host on ports $ports_csv"
  PYTHONUNBUFFERED=1 nohup python3 "$RELAY_SCRIPT" \
    --listen-host "$listen_host" \
    --target-host "$target_host" \
    --ports "$ports_csv" \
    --log-dir "$log_dir" \
    >"$STDOUT_LOG" 2>"$STDERR_LOG" < /dev/null &
  local pid=$!
  printf '%s\n' "$pid" > "$PID_FILE"

  sleep 1
  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$PID_FILE"
    die "relay failed to start; check $STDERR_LOG"
  fi
  log "relay started with pid $pid"
}

stop_relay() {
  cleanup_stale_pid
  if ! is_running; then
    log 'relay is not running'
    rm -f "$PID_FILE"
    return 0
  fi

  local pid
  pid=$(relay_pid)
  log "stopping relay pid $pid"
  kill "$pid"
  local waited=0
  while kill -0 "$pid" 2>/dev/null; do
    sleep 1
    waited=$((waited + 1))
    if (( waited >= 10 )); then
      log "relay did not stop cleanly; sending SIGKILL"
      kill -9 "$pid" 2>/dev/null || true
      break
    fi
  done
  rm -f "$PID_FILE"
}

status() {
  load_state_if_present
  cleanup_stale_pid
  normalize_ports

  if is_running; then
    log "status: running (pid $(relay_pid))"
  else
    log 'status: stopped'
  fi
  log "target_host=$target_host"
  log "allow_from=$allow_from"
  log "ports=$ports_csv"
  log "log_dir=$log_dir"
  log "stdout_log=$STDOUT_LOG"
  log "stderr_log=$STDERR_LOG"
}

run_on() {
  normalize_ports
  open_ports
  start_relay
  status
}

run_off() {
  load_state_if_present
  normalize_ports
  stop_relay
  close_ports
  status
}

parse_args "$@"
cleanup_stale_pid

case "$command" in
  on)
    run_on
    ;;
  off)
    run_off
    ;;
  toggle)
    load_state_if_present
    normalize_ports
    if is_running; then
      run_off
    else
      run_on
    fi
    ;;
  status)
    status
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    die "unknown command: $command"
    ;;
esac
