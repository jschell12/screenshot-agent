#!/usr/bin/env bash
set -euo pipefail

DISCOVERY_SECONDS="${DISCOVERY_SECONDS:-4}"
DEFAULT_USER="${SSH_USER:-$(whoami)}"

REMOTE_HOST=""
REMOTE_LABEL=""

ssh_opts=(
  -o ExitOnForwardFailure=yes
  -o ServerAliveInterval=60
  -o ServerAliveCountMax=3
)

usage() {
  cat <<'EOF'
Usage:
  mac-link.sh tunnel  [remote_host] [local_port]  [remote_port] [ssh_user]
  mac-link.sh rtunnel [remote_host] [remote_port] [local_port]  [ssh_user]
  mac-link.sh push    [remote_host] [src]         [dest_path]   [ssh_user]
  mac-link.sh pull    [remote_host] [remote_path] [dest_path]   [ssh_user]
  mac-link.sh discover

If remote_host is omitted, the script will discover Macs on your LAN that
advertise SSH and let you pick one.

Examples:
  ./mac-link.sh tunnel
  ./mac-link.sh tunnel macmini.local 15432 5432 josh
  ./mac-link.sh push
  ./mac-link.sh pull
  ./mac-link.sh discover
EOF
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

quote_remote_path() {
  printf "'%s'" "$(printf "%s" "$1" | sed "s/'/'\\\\''/g")"
}

discover_ssh_services() {
  local tmp pid
  tmp="$(mktemp)"
  dns-sd -B _ssh._tcp local. >"$tmp" 2>/dev/null &
  pid=$!

  sleep "$DISCOVERY_SECONDS"
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" 2>/dev/null || true

  awk '
    / Add / && /_ssh\._tcp\./ {
      name = ""
      for (i = 7; i <= NF; i++) {
        name = name (i == 7 ? "" : OFS) $i
      }
      if (name != "") print name
    }
  ' "$tmp" | sort -u

  rm -f "$tmp"
}

resolve_service_to_host() {
  local instance="$1"
  local tmp pid
  tmp="$(mktemp)"
  dns-sd -L "$instance" _ssh._tcp local. >"$tmp" 2>/dev/null &
  pid=$!

  sleep 2
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" 2>/dev/null || true

  awk '
    /can be reached at/ {
      sub(/^.*can be reached at /, "", $0)
      sub(/:[0-9]+.*$/, "", $0)
      sub(/\.$/, "", $0)
      print
      exit
    }
  ' "$tmp"

  rm -f "$tmp"
}

choose_remote_host() {
  local names=()
  local hosts=()
  local line host i choice idx

  while IFS= read -r line; do
    [[ -n "$line" ]] && names+=("$line")
  done < <(discover_ssh_services)

  if [[ ${#names[@]} -eq 0 ]]; then
    echo "No SSH-advertising Macs were discovered on the LAN."
    read -r -p "Enter hostname or IP manually: " REMOTE_HOST
    REMOTE_LABEL="$REMOTE_HOST"
    [[ -n "$REMOTE_HOST" ]] || exit 1
    return
  fi

  echo "Discovered SSH hosts:"
  i=1
  for line in "${names[@]}"; do
    host="$(resolve_service_to_host "$line")"
    [[ -n "$host" ]] || host="$line"
    hosts+=("$host")
    printf "  [%d] %s -> %s\n" "$i" "$line" "$host"
    i=$((i + 1))
  done
  echo "  [m] Enter hostname or IP manually"

  while true; do
    read -r -p "Choose target: " choice
    case "$choice" in
      [mM])
        read -r -p "Enter hostname or IP: " REMOTE_HOST
        REMOTE_LABEL="$REMOTE_HOST"
        [[ -n "$REMOTE_HOST" ]] || continue
        return
        ;;
      ''|*[!0-9]*)
        echo "Enter a number or m."
        ;;
      *)
        idx=$((choice - 1))
        if [[ $idx -ge 0 && $idx -lt ${#hosts[@]} ]]; then
          REMOTE_HOST="${hosts[$idx]}"
          REMOTE_LABEL="${names[$idx]}"
          return
        fi
        echo "Invalid selection."
        ;;
    esac
  done
}

pick_or_use_host() {
  local maybe_host="${1:-}"
  if [[ -n "$maybe_host" ]]; then
    REMOTE_HOST="$maybe_host"
    REMOTE_LABEL="$maybe_host"
  else
    choose_remote_host
  fi
}

prompt_default() {
  local prompt="$1"
  local default="$2"
  local value
  read -r -p "$prompt [$default]: " value
  printf "%s" "${value:-$default}"
}

do_tunnel() {
  local remote_host="$1"
  local local_port="$2"
  local remote_port="$3"
  local ssh_user="$4"

  echo "Opening tunnel:"
  echo "  localhost:${local_port} -> ${remote_host}:127.0.0.1:${remote_port}"
  exec ssh "${ssh_opts[@]}" -N \
    -L "${local_port}:127.0.0.1:${remote_port}" \
    "${ssh_user}@${remote_host}"
}

do_rtunnel() {
  local remote_host="$1"
  local remote_port="$2"
  local local_port="$3"
  local ssh_user="$4"

  echo "Opening reverse tunnel:"
  echo "  ${remote_host}:127.0.0.1:${remote_port} -> this-mac:127.0.0.1:${local_port}"
  exec ssh "${ssh_opts[@]}" -N \
    -R "${remote_port}:127.0.0.1:${local_port}" \
    "${ssh_user}@${remote_host}"
}

do_push() {
  local remote_host="$1"
  local src="$2"
  local dest_path="$3"
  local ssh_user="$4"
  local quoted_dest

  quoted_dest="$(quote_remote_path "$dest_path")"

  echo "Creating remote path if needed..."
  ssh "${ssh_opts[@]}" "${ssh_user}@${remote_host}" "mkdir -p ${quoted_dest}"

  echo "Syncing local -> remote"
  rsync -avh --progress --partial \
    -e "ssh ${ssh_opts[*]}" \
    -- "$src" "${ssh_user}@${remote_host}:${quoted_dest}"
}

do_pull() {
  local remote_host="$1"
  local remote_path="$2"
  local dest_path="$3"
  local ssh_user="$4"
  local quoted_remote

  quoted_remote="$(quote_remote_path "$remote_path")"

  mkdir -p "$dest_path"

  echo "Syncing remote -> local"
  rsync -avh --progress --partial \
    -e "ssh ${ssh_opts[*]}" \
    -- "${ssh_user}@${remote_host}:${quoted_remote}" "$dest_path"
}

interactive_tunnel() {
  local ssh_user local_port remote_port
  ssh_user="$(prompt_default "SSH user" "$DEFAULT_USER")"
  read -r -p "Local port to open on this Mac: " local_port
  read -r -p "Remote port on ${REMOTE_HOST}: " remote_port
  do_tunnel "$REMOTE_HOST" "$local_port" "$remote_port" "$ssh_user"
}

interactive_rtunnel() {
  local ssh_user remote_port local_port
  ssh_user="$(prompt_default "SSH user" "$DEFAULT_USER")"
  read -r -p "Remote port to open on ${REMOTE_HOST}: " remote_port
  read -r -p "Local port on this Mac: " local_port
  do_rtunnel "$REMOTE_HOST" "$remote_port" "$local_port" "$ssh_user"
}

interactive_push() {
  local ssh_user src dest_path
  ssh_user="$(prompt_default "SSH user" "$DEFAULT_USER")"
  read -r -p "Local source path: " src
  read -r -p "Destination path on ${REMOTE_HOST}: " dest_path
  do_push "$REMOTE_HOST" "$src" "$dest_path" "$ssh_user"
}

interactive_pull() {
  local ssh_user remote_path dest_path
  ssh_user="$(prompt_default "SSH user" "$DEFAULT_USER")"
  read -r -p "Remote source path on ${REMOTE_HOST}: " remote_path
  read -r -p "Local destination path: " dest_path
  do_pull "$REMOTE_HOST" "$remote_path" "$dest_path" "$ssh_user"
}

interactive_menu() {
  local choice
  choose_remote_host
  echo "Selected: ${REMOTE_LABEL} (${REMOTE_HOST})"
  echo
  echo "  [1] Open local tunnel"
  echo "  [2] Open reverse tunnel"
  echo "  [3] Push files"
  echo "  [4] Pull files"

  while true; do
    read -r -p "Choose action: " choice
    case "$choice" in
      1) interactive_tunnel; break ;;
      2) interactive_rtunnel; break ;;
      3) interactive_push; break ;;
      4) interactive_pull; break ;;
      *) echo "Enter 1, 2, 3, or 4." ;;
    esac
  done
}

need_cmd ssh
need_cmd rsync
need_cmd dns-sd

[[ $# -ge 1 ]] || {
  interactive_menu
  exit 0
}

cmd="$1"
shift || true

case "$cmd" in
  discover)
    choose_remote_host
    echo "${REMOTE_HOST}"
    ;;
  tunnel)
    pick_or_use_host "${1:-}"
    local_port="${2:-}"
    remote_port="${3:-}"
    ssh_user="${4:-$DEFAULT_USER}"

    [[ -n "$local_port" ]] || read -r -p "Local port to open on this Mac: " local_port
    [[ -n "$remote_port" ]] || read -r -p "Remote port on ${REMOTE_HOST}: " remote_port

    do_tunnel "$REMOTE_HOST" "$local_port" "$remote_port" "$ssh_user"
    ;;
  rtunnel)
    pick_or_use_host "${1:-}"
    remote_port="${2:-}"
    local_port="${3:-}"
    ssh_user="${4:-$DEFAULT_USER}"

    [[ -n "$remote_port" ]] || read -r -p "Remote port to open on ${REMOTE_HOST}: " remote_port
    [[ -n "$local_port" ]] || read -r -p "Local port on this Mac: " local_port

    do_rtunnel "$REMOTE_HOST" "$remote_port" "$local_port" "$ssh_user"
    ;;
  push)
    pick_or_use_host "${1:-}"
    src="${2:-}"
    dest_path="${3:-}"
    ssh_user="${4:-$DEFAULT_USER}"

    [[ -n "$src" ]] || read -r -p "Local source path: " src
    [[ -n "$dest_path" ]] || read -r -p "Destination path on ${REMOTE_HOST}: " dest_path

    do_push "$REMOTE_HOST" "$src" "$dest_path" "$ssh_user"
    ;;
  pull)
    pick_or_use_host "${1:-}"
    remote_path="${2:-}"
    dest_path="${3:-}"
    ssh_user="${4:-$DEFAULT_USER}"

    [[ -n "$remote_path" ]] || read -r -p "Remote source path on ${REMOTE_HOST}: " remote_path
    [[ -n "$dest_path" ]] || read -r -p "Local destination path: " dest_path

    do_pull "$REMOTE_HOST" "$remote_path" "$dest_path" "$ssh_user"
    ;;
  *)
    usage
    ;;
esac
